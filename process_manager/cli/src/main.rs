use pm_engine::proto::process_manager_client::ProcessManagerClient;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let channel = pm_engine::transport::create_channel_default().await?;
    let _client = ProcessManagerClient::new(channel);
    println!("No RPCs defined yet; client connected.");

    Ok(())
}
