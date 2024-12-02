from time import sleep

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.issue.assign import assign_with_rules
from tasks.libs.issue.model.constants import BASE_MODEL, MODEL, TEAMS


def fetch_data_and_train_model():
    gh = GithubAPI('DataDog/datadog-agent')
    d = gh.repo
    issues = []
    teams = []
    for id, issue in enumerate(d.get_issues(state='all')):
        issues.append(f"{issue.title} {issue.body}".casefold())
        teams.append(assign_with_rules(issue, gh))
        # Sleep to avoid hitting the rate limit
        if id % 2000 == 0:
            sleep(3600)

    train_the_model(teams, issues, "issue_auto_assign_model", 64, 5)


def train_the_model(teams, issues, batch_size, epochs):
    import torch
    from sklearn.model_selection import train_test_split
    from torch.utils.data import DataLoader, Dataset
    from transformers import AutoModelForSequenceClassification, AutoTokenizer

    class IssueDataset(Dataset):
        def __init__(self, issues, labels, tokenizer, max_length=64):
            self.issues = issues
            self.labels = labels
            self.tokenizer = tokenizer
            self.max_length = max_length

        def __len__(self):
            return len(self.issues)

        def __getitem__(self, idx):
            issue = self.issues[idx]
            label = self.labels[idx]
            inputs = self.tokenizer(
                issue, max_length=self.max_length, padding="max_length", truncation=True, return_tensors="pt"
            )
            return {
                "input_ids": inputs["input_ids"].flatten(),
                "attention_mask": inputs["attention_mask"].flatten(),
                "labels": torch.tensor(label, dtype=torch.long),
            }

    # Split the dataset into training and validation sets
    train_issues, val_issues, train_teams, val_teams = train_test_split(issues, teams, test_size=0.2, random_state=42)

    # Define hyperparameters
    learning_rate = 2e-5

    # Load pre-trained BERT model and tokenizer
    tokenizer = AutoTokenizer.from_pretrained(BASE_MODEL)
    model = AutoModelForSequenceClassification.from_pretrained(
        BASE_MODEL, num_labels=len(set(teams)), ignore_mismatched_sizes=True
    )

    # Prepare dataset and dataloaders
    train_teams = [TEAMS.index(t) for t in train_teams]
    val_teams = [TEAMS.index(t) for t in val_teams]
    train_dataset = IssueDataset(train_issues, train_teams, tokenizer, max_length=batch_size)
    val_dataset = IssueDataset(val_issues, val_teams, tokenizer, max_length=batch_size)
    print(f"set sizes : {len(train_dataset)} {len(val_dataset)} {len(set(teams))}")

    print(f"train_dataset {train_dataset[0]}")
    print(f"train_dataset {train_dataset[1]}")
    train_loader = DataLoader(train_dataset, batch_size=batch_size, shuffle=True)
    val_loader = DataLoader(val_dataset, batch_size=batch_size)

    # Define optimizer and loss function
    optimizer = torch.optim.AdamW(model.parameters(), lr=learning_rate)

    print("Start training...")
    # Fine-tune the model
    for epoch in range(epochs):
        print(f"Epoch {epoch+1}/{epochs}")
        model.train()
        train_loss = 0.0
        for batch in train_loader:
            optimizer.zero_grad()
            input_ids, attention_mask, labels = batch['input_ids'], batch['attention_mask'], batch['labels']
            outputs = model(input_ids=input_ids, attention_mask=attention_mask, labels=labels)
            loss = outputs.loss
            train_loss += loss.item()
            loss.backward()
            optimizer.step()
        train_loss /= len(train_loader)

        # Evaluate on validation set
        model.eval()
        val_loss = 0.0
        correct = 0
        total = 0
        print("validate")
        with torch.no_grad():
            for batch in val_loader:
                input_ids, attention_mask, labels = batch['input_ids'], batch['attention_mask'], batch['labels']
                outputs = model(input_ids=input_ids, attention_mask=attention_mask, labels=labels)
                loss = outputs.loss
                val_loss += loss.item()
                _, predicted = torch.max(outputs.logits, 1)
                total += labels.size(0)
                correct += (predicted == labels).sum().item()
        val_loss /= len(val_loader)
        val_accuracy = correct / total

        print(
            f"Epoch {epoch+1}/{epochs}: Train Loss: {train_loss:.4f}, Val Loss: {val_loss:.4f}, Val Accuracy: {val_accuracy:.4f}"
        )
    model.save_pretrained(MODEL)
