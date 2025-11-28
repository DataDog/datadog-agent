//! Demonstration of the process manager architecture with use cases
//! Run with: cargo run --example use_case_demo

use pm_engine::application::Application;
use pm_engine::domain::CreateProcessCommand;
use pm_engine::infrastructure::{InMemoryProcessRepository, TokioProcessExecutor};
use std::sync::Arc;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    println!("Process Manager - Use Case Demo\n");
    println!("==========================================\n");

    // 1. Create infrastructure layer (real adapters!)
    let repository = Arc::new(InMemoryProcessRepository::new());
    let executor = Arc::new(TokioProcessExecutor::new());
    println!("[OK] Infrastructure Layer: Created InMemoryRepository & TokioExecutor");

    // 2. Create application layer (use case registry)
    let use_cases = Application::new(repository.clone(), executor.clone());
    println!("[OK] Application Layer: Created Application with all 8 use cases\n");

    println!("==========================================");
    println!("CQRS Pattern Demonstration");
    println!("==========================================\n");

    // 3. Query: List processes (should be empty initially)
    println!("Query: Listing all processes...");
    let list_result = use_cases.list_processes().execute().await?;
    println!(
        "   Found {} processes (initially empty)\n",
        list_result.processes.len()
    );

    // 4. Command: Create first process
    println!("Command: Creating 'web-server' process...");
    let create_command = CreateProcessCommand {
        name: "web-server".to_string(),
        command: "/usr/bin/nginx".to_string(),
        args: vec![],
        ..Default::default()
    };
    let created = use_cases.create_process().execute(create_command).await?;
    println!("   [OK] Created: {} (ID: {})", created.name, created.id);

    // 5. Command: Create second process
    println!("\nCommand: Creating 'api-server' process...");
    let create_command = CreateProcessCommand {
        name: "api-server".to_string(),
        command: "/usr/bin/node server.js".to_string(),
        args: vec![],
        ..Default::default()
    };
    let created = use_cases.create_process().execute(create_command).await?;
    println!("   [OK] Created: {} (ID: {})", created.name, created.id);

    // 6. Command: Try to create duplicate (should fail)
    println!("\nCommand: Attempting to create duplicate 'web-server'...");
    let duplicate_command = CreateProcessCommand {
        name: "web-server".to_string(),
        command: "/usr/bin/nginx".to_string(),
        args: vec![],
        ..Default::default()
    };
    match use_cases.create_process().execute(duplicate_command).await {
        Ok(_) => println!("   [FAIL] Unexpected: Duplicate was allowed!"),
        Err(e) => println!("   [OK] Validation working: {}", e),
    }

    // 7. Query: List all processes
    println!("\nQuery: Listing all processes...");
    let list_result = use_cases.list_processes().execute().await?;
    println!("   Found {} processes:", list_result.processes.len());
    for process in &list_result.processes {
        println!("   - {} (ID: {})", process.name(), process.id());
    }

    println!("\n==========================================");
    println!("[OK] Demo completed successfully!");
    println!("==========================================\n");

    println!("Architecture Benefits Demonstrated:\n");
    println!("   - Clean separation: Domain -> Application -> Infrastructure");
    println!("   - CQRS: Commands (Create) and Queries (List)");
    println!("   - Dependency Inversion: Use cases depend on ports, not implementations");
    println!("   - Real Infrastructure: TokioExecutor & InMemoryRepository");
    println!("   - Validation: Business rules enforced in domain layer");
    println!("   - Type Safety: ProcessId value object prevents primitive obsession\n");

    println!("Complete Use Case Inventory:\n");
    println!("   Commands: Create, Start, Stop, Restart, Update, Delete");
    println!("   Queries: List, GetStatus");
    println!("   Total: 8 use cases fully implemented!\n");

    println!("Ready for:\n");
    println!("   -> gRPC/REST adapters using the registry");
    println!("   -> Connect to existing ProcessRegistryManager");
    println!("   -> Full end-to-end process lifecycle tests");
    println!("   -> Production deployment with real system processes");

    Ok(())
}
