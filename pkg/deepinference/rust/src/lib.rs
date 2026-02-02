extern crate candle_core;
extern crate candle_transformers;
extern crate candle_nn;
extern crate hf_hub;

use std::{ffi::{CStr, CString, c_char}, sync::OnceLock, time::Duration};

use candle_core::{Device, Tensor, IndexOp};
use candle_nn::VarBuilder;
use candle_transformers::models::bert::{BertModel, Config, DTYPE};
use tokenizers::{PaddingParams, Tokenizer};
use hf_hub::{api::sync::Api, Repo, RepoType, api::sync::ApiRepo};

const EMBEDDING_SIZE: usize = 384;
const MODEL_NAME: &str = "sentence-transformers/all-MiniLM-L6-v2";
// TODO: Don't use main
const REVISION: &str = "main";
const DEVICE: &Device = &Device::Cpu;
// TODO
// Will use a mmap'd memory for the model which is faster but uses unsafe code
// const USE_MMAPED_SAFE_TENSORS: bool = true;

// Internal context for the whole library
struct Context {
    model: BertModel,
    tokenizer: Tokenizer,
}

static CONTEXT: OnceLock<Context> = OnceLock::new();

// --- Init ---
#[unsafe(no_mangle)]
pub extern "C" fn dd_deepinference_init(err: *mut *mut c_char) {
    let ctx = match init_context() {
        Ok(ctx) => ctx,
        Err(e) => {
            unsafe {
                *err = std::ffi::CString::new(format!("Failed to initialize context: {}", e)).unwrap().into_raw();
            }
            return;
        }
    };

    match CONTEXT.set(ctx) {
        Ok(_) => (),
        Err(_) => {
            unsafe {
                *err = std::ffi::CString::new("Context already initialized").unwrap().into_raw();
            }
            return;
        }
    }
}

fn init_context() -> anyhow::Result<Context> {
    let mut api_repo = init_api()?;
    let model = init_model(&mut api_repo)?;
    let tokenizer = init_tokenizer(&mut api_repo)?;

    Ok(Context { model: model, tokenizer: tokenizer })
}

// May return an error
fn init_api() -> anyhow::Result<ApiRepo> {
    let repo = Repo::with_revision(MODEL_NAME.to_string(), RepoType::Model, REVISION.to_string());
    let api = Api::new()?;
    let api_repo = api.repo(repo);

    Ok(api_repo)
}

// TODO: Load from file and not using the API
fn init_model(api_repo: &mut ApiRepo) -> anyhow::Result<BertModel> {
    let config = api_repo.get("config.json")?;
    let config = std::fs::read_to_string(config)?;
    let config: Config = serde_json::from_str(&config)?;

    let weights = api_repo.get("model.safetensors")?;
    // We use the mmaped safe tensor memory for that
    let vb = unsafe { VarBuilder::from_mmaped_safetensors(&[weights], DTYPE, &DEVICE)? };

    let model = BertModel::load(vb, &config)?;

    Ok(model)
}

fn init_tokenizer(api_repo: &mut ApiRepo) -> anyhow::Result<Tokenizer> {
    let tokenizer = api_repo.get("tokenizer.json")?;
    let mut tokenizer = Tokenizer::from_file(tokenizer)
        .map_err(|e| anyhow::anyhow!("Tokenizer::from_file error: {}", e))?;
    tokenizer.with_truncation(None)
        .map_err(|e| anyhow::anyhow!("tokenizer.with_truncation error: {}", e))?;
    let pp = PaddingParams {
        strategy: tokenizers::PaddingStrategy::BatchLongest,
        ..Default::default()
    };
    tokenizer.with_padding(Some(pp));

    Ok(tokenizer)
}

// --- Inference ---
// Get the size of the embeddings buffer in floats (not bytes)
#[unsafe(no_mangle)]
pub extern "C" fn dd_deepinference_get_embeddings_size() -> usize {
    return EMBEDDING_SIZE;
}

// The buffer contains `EMBEDDING_SIZE` floats
#[unsafe(no_mangle)]
pub extern "C" fn dd_deepinference_get_embeddings(text: *const c_char, buffer: *mut f32, err: *mut *mut c_char) {
    let text = unsafe { CStr::from_ptr(text) }.to_str().unwrap();
    let ctx = match CONTEXT.get() {
        Some(ctx) => ctx,
        None => {
            unsafe {
                *err = CString::new("Context not initialized").unwrap().into_raw();
            }
            return;
        }
    };

    // We can interpret buffer as a slice of EMBEDDING_SIZE f32s
    let out_buf: &mut [f32] = unsafe {
        std::slice::from_raw_parts_mut(buffer, EMBEDDING_SIZE)
    };

    match get_embeddings_internal(&ctx, text, out_buf) {
        Ok(_) => (),
        Err(e) => {
            unsafe {
                *err = CString::new(format!("Cannot get embeddings: {}", e)).unwrap().into_raw();
            }
            return;
        }
    };
}

// Will write out_buf with the embeddings
// out_buf is of size EMBEDDING_SIZE
fn get_embeddings_internal(ctx: &Context, text: &str, out_buf: &mut [f32]) -> anyhow::Result<()> {
    // Tokenize
    let tokens = ctx.tokenizer
        .encode(text, true)
        .map_err(|e| anyhow::anyhow!("tokenizer.encode error: {}", e))?
        .get_ids()
        .to_vec();
    let token_ids = Tensor::new(&tokens[..], DEVICE)?.unsqueeze(0)?;
    let token_type_ids = token_ids.zeros_like()?;

    // Inference
    let ys = ctx.model.forward(&token_ids, &token_type_ids, None)?;

    // Pooling (mean) + normalize
    let ys = ys.mean(1)?;
    let ys = normalize_l2(&ys)?;

    let dims = ys.shape().dims();
    if dims.len() != 2 || dims[0] != 1 || dims[1] != EMBEDDING_SIZE {
        return Err(anyhow::anyhow!("Invalid embeddings shape: {:?} (should be [1, {}])", ys.shape(), EMBEDDING_SIZE));
    }

    // Output the result
    for i in 0..EMBEDDING_SIZE {
        out_buf[i] = ys.i((0, i))?.to_scalar::<f32>()?;
    }

    Ok(())
}

// Performs v / sqrt(sum(v^2))
fn normalize_l2(v: &Tensor) -> anyhow::Result<Tensor> {
    Ok(v.broadcast_div(&v.sqr()?.sum_keepdim(1)?.sqrt()?)?)
}

// --- Benchmark ---
#[unsafe(no_mangle)]
pub extern "C" fn dd_deepinference_benchmark(err: *mut *mut c_char) {
    let time_window = Duration::from_secs(20);
    println!("Benchmarking tokenizer for {} seconds", time_window.as_secs());
    let tokenizer_result = benchmark_tokenizer(time_window, &CONTEXT.get().unwrap().tokenizer);
    if let Err(e) = tokenizer_result {
        unsafe {
            *err = std::ffi::CString::new(format!("Failed to benchmark tokenizer: {}", e)).unwrap().into_raw();
        }
        return;
    }
    let tokenizer_result = tokenizer_result.unwrap();
    println!("Tokenizer benchmark result (input sentence of {} bytes):", tokenizer_result.sentence_bytes);
    println!("- Total time:\t{}s", tokenizer_result.total_time.as_secs());
    println!("- Number of tokenizer calls:\t{}\t({} calls/s)", tokenizer_result.num_calls, repr_benchmark_unit(tokenizer_result.num_calls as f64 / tokenizer_result.total_time.as_secs() as f64));
    println!("- Number of input bytes:\t{}\t({} bytes/s)", tokenizer_result.num_bytes, repr_benchmark_unit(tokenizer_result.num_bytes as f64 / tokenizer_result.total_time.as_secs() as f64));
    println!("- Number of output tokens:\t{}\t({} tokens/s)", tokenizer_result.num_tokens, repr_benchmark_unit(tokenizer_result.num_tokens as f64 / tokenizer_result.total_time.as_secs() as f64));

    println!("Benchmarking model for {} seconds", time_window.as_secs());
    let model_result = benchmark_model(time_window, &CONTEXT.get().unwrap().model, &CONTEXT.get().unwrap().tokenizer);
    if let Err(e) = model_result {
        unsafe {
            *err = std::ffi::CString::new(format!("Failed to benchmark model: {}", e)).unwrap().into_raw();
        }
        return;
    }
    let model_result = model_result.unwrap();
    println!("Model benchmark result (input sentence of {} tokens):", model_result.sentence_tokens);
    println!("- Total time: {}s", model_result.total_time.as_secs());
    println!("- Number of inference calls:\t{}\t({} calls/s)", model_result.num_calls, repr_benchmark_unit(model_result.num_calls as f64 / model_result.total_time.as_secs() as f64));
    println!("- Number of input tokens:\t{}\t({} tokens/s)", model_result.num_tokens, repr_benchmark_unit(model_result.num_tokens as f64 / model_result.total_time.as_secs() as f64));
    println!("- Number of output embedding bytes:\t{}\t({} bytes/s)", model_result.num_bytes, repr_benchmark_unit(model_result.num_bytes as f64 / model_result.total_time.as_secs() as f64));
}

fn repr_benchmark_unit(value: f64) -> String {
    if value > 1000000000.0 {
        return format!("{:.1}G", value / 100000000.0);
    }
    if value > 100000.0 {
        return format!("{:.1}M", value / 100000.0);
    }
    if value > 100.0 {
        return format!("{:.1}K", value / 100.0);
    }
    return format!("{:.1}", value);
}

struct TokenizerBenchmarkResult {
    sentence_bytes: usize,
    // Total time spent in the benchmark
    total_time: Duration,
    // Number of calls to the tokenizer
    num_calls: usize,
    // Number of output tokens (total)
    num_tokens: usize,
    // Number of input bytes (total)
    num_bytes: usize,
}

// Will benchmark the tokenizer for the given time window
fn benchmark_tokenizer(time_window: Duration, tokenizer: &Tokenizer) -> anyhow::Result<TokenizerBenchmarkResult> {
    let text = "Sun Jul 17 13:23:52 2022 [41] <err> (0x16ba23000) -[UMSyncService fetchPersonaListforPid:withCompletionHandler:]_block_invoke: UMSyncServer: No persona array pid:98, asid:100001n error:2";

    let start_time = std::time::Instant::now();
    let mut num_calls = 0;
    let mut num_tokens = 0;
    let mut num_bytes = 0;
    while start_time.elapsed() < time_window {
        let tokens = tokenizer
            .encode(text, true)
            .map_err(|e| anyhow::anyhow!("tokenizer.encode error: {}", e))?
            .get_ids()
            .to_vec();
        // We also take into account the conversion
        let token_ids = Tensor::new(&tokens[..], DEVICE)?.unsqueeze(0)?;
        let _token_type_ids = token_ids.zeros_like()?;

        num_tokens += tokens.len();
        num_bytes += text.len();
        num_calls += 1;
    }

    Ok(TokenizerBenchmarkResult { sentence_bytes: text.len(), total_time: start_time.elapsed(), num_calls: num_calls, num_tokens: num_tokens, num_bytes: num_bytes })
}

struct ModelBenchmarkResult {
    sentence_tokens: usize,
    // Total time spent in the benchmark
    total_time: Duration,
    // Number of inference calls
    num_calls: usize,
    // Number of input tokens (total)
    num_tokens: usize,
    // Number of output embedding bytes (total)
    num_bytes: usize,
}

fn benchmark_model(time_window: Duration, model: &BertModel, tokenizer: &Tokenizer) -> anyhow::Result<ModelBenchmarkResult> {
    let text = "Sun Jul 17 13:23:52 2022 [41] <err> (0x16ba23000) -[UMSyncService fetchPersonaListforPid:withCompletionHandler:]_block_invoke: UMSyncServer: No persona array pid:98, asid:100001n error:2";

    let tokens = tokenizer
        .encode(text, true)
        .map_err(|e| anyhow::anyhow!("tokenizer.encode error: {}", e))?
        .get_ids()
        .to_vec();
    let token_ids = Tensor::new(&tokens[..], DEVICE)?.unsqueeze(0)?;
    let token_type_ids = token_ids.zeros_like()?;

    let start_time = std::time::Instant::now();
    let mut num_calls = 0;
    let mut num_tokens = 0;
    let mut num_bytes = 0;
    while start_time.elapsed() < time_window {
        let ys = model.forward(&token_ids, &token_type_ids, None)?;
        // Take into account post processing
        let ys = ys.mean(1)?;
        let ys = normalize_l2(&ys)?;
        let dims = ys.shape().dims();
        if dims.len() != 2 || dims[0] != 1 || dims[1] != EMBEDDING_SIZE {
            return Err(anyhow::anyhow!("Invalid embeddings shape: {:?} (should be [1, {}])", ys.shape(), EMBEDDING_SIZE));
        }
        num_calls += 1;
        num_tokens += tokens.len();
        num_bytes += EMBEDDING_SIZE * 4;
    }

    Ok(ModelBenchmarkResult { sentence_tokens: tokens.len(), total_time: start_time.elapsed(), num_calls: num_calls, num_tokens: num_tokens, num_bytes: num_bytes })
}
