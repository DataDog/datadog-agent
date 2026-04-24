#!/usr/bin/env python3
"""
Export sentiment and category models to ONNX format for the Rust native host.

This script exports:
1. Seethal/sentiment_analysis_generic_dataset - 3-class sentiment (positive/neutral/negative)
2. typeform/mobilebert-uncased-mnli - Zero-shot classification for categories

Run once to generate the ONNX models that the Rust host will download.
"""

import sys
from pathlib import Path

try:
    from optimum.exporters.onnx import main_export
    from transformers import AutoModelForSequenceClassification, AutoTokenizer
except ImportError:
    print("Please install: pip install optimum[exporters] transformers torch")
    sys.exit(1)

OUTPUT_DIR = Path(__file__).parent.parent / "models"

MODELS = {
    "sentiment": {
        "model_id": "Seethal/sentiment_analysis_generic_dataset",
        "task": "text-classification",
    },
    "category": {
        "model_id": "typeform/mobilebert-uncased-mnli",
        "task": "zero-shot-classification",
    },
}


def export_model(name: str, model_id: str, task: str) -> None:
    """Export a HuggingFace model to ONNX format."""
    output_path = OUTPUT_DIR / name
    output_path.mkdir(parents=True, exist_ok=True)

    print(f"\n{'='*60}")
    print(f"Exporting {name}: {model_id}")
    print(f"Task: {task}")
    print(f"Output: {output_path}")
    print('=' * 60)

    main_export(
        model_name_or_path=model_id,
        output=output_path,
        task=task,
        opset=14,
    )

    print(f"Successfully exported {name} to {output_path}")


def verify_sentiment_labels():
    """Verify the sentiment model has 3 classes."""
    model_id = MODELS["sentiment"]["model_id"]
    print(f"\nVerifying sentiment model labels: {model_id}")

    model = AutoModelForSequenceClassification.from_pretrained(model_id)
    AutoTokenizer.from_pretrained(model_id)

    labels = model.config.id2label
    print(f"Labels: {labels}")

    if len(labels) != 3:
        print(f"WARNING: Expected 3 labels, got {len(labels)}")
    else:
        print("Confirmed: 3-class sentiment model")

    return labels


def main():
    print("ONNX Model Export Script for Rust Native Host")
    print("=" * 60)

    OUTPUT_DIR.mkdir(parents=True, exist_ok=True)

    labels = verify_sentiment_labels()

    for name, config in MODELS.items():
        export_model(name, config["model_id"], config["task"])

    print("\n" + "=" * 60)
    print("Export complete!")
    print(f"Models saved to: {OUTPUT_DIR}")
    print("\nSentiment labels mapping:")
    for idx, label in labels.items():
        print(f"  {idx} -> {label}")


if __name__ == "__main__":
    main()
