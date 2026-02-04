// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

#include <iostream>
#include <vector>
#include <cmath>
#include <string>
#include <cstring>
#include <cstdlib>

#include "onnxruntime_cxx_api.h"

// Default model path - can be overridden via environment variable or command line
const char* DEFAULT_MODEL_PATH = nullptr; // Will be set from Hugging Face cache location

// Hello, world!
// const std::vector<int64_t> EXAMPLE_INPUT_IDS = {101, 7592, 1010, 2088, 999, 102};
// Sun Jul 17 13:23:52 2022 [41] <err> (0x16ba23000) -[UMSyncService fetchPersonaListforPid:withCompletionHandler:]_block_invoke: UMSyncServer: No persona array pid:98, asid:100001n error:2
const std::vector<int64_t> EXAMPLE_INPUT_IDS = {101, 3103, 21650, 2459, 2410, 1024, 2603, 1024, 4720, 16798, 2475, 1031, 4601, 1033, 1026, 9413, 2099, 1028, 1006, 1014, 2595, 16048, 3676, 21926, 8889, 2692, 1007, 1011, 1031, 8529, 6508, 12273, 8043, 7903, 2063, 18584, 28823, 2923, 29278, 23267, 1024, 2007, 9006, 10814, 3508, 11774, 3917, 1024, 1033, 1035, 3796, 1035, 1999, 6767, 3489, 1024, 8529, 6508, 12273, 8043, 6299, 1024, 2053, 16115, 9140, 14255, 2094, 1024, 5818, 1010, 2004, 3593, 1024, 6694, 24096, 2078, 7561, 1024, 1016, 102};
const int64_t BATCH_SIZE = 1;
const int64_t SEQUENCE_LENGTH = 6;

std::string getDefaultModelPath() {
    // Try to get from environment variable first
    const char* env_path = std::getenv("ONNX_MODEL_PATH");
    if (env_path != nullptr) {
        return std::string(env_path);
    }

    const char* home = std::getenv("HOME");

    // Note: Hugging Face cache uses hash-based snapshot directories
    // Run download_model.py first to get the exact path, then use:
    //   ./onnx_inference /path/to/model_O4.onnx
    //   or export ONNX_MODEL_PATH=/path/to/model_O4.onnx
    // Fallback - user should provide actual path
    return std::string(home) + "/.cache/huggingface/hub/models--sentence-transformers--all-MiniLM-L6-v2/snapshots/c9745ed1d9f207416be6d2e6f8de32d1f16199bf/onnx/model_O4.onnx";
    // return std::string(home) + "/.cache/huggingface/hub/models--sentence-transformers--all-MiniLM-L6-v2/snapshots/c9745ed1d9f207416be6d2e6f8de32d1f16199bf/onnx/model.onnx";
    // return std::string(home) + "/.cache/huggingface/hub/models--sentence-transformers--all-MiniLM-L6-v2/snapshots/c9745ed1d9f207416be6d2e6f8de32d1f16199bf/onnx/model_qint8_arm64.onnx";
}

std::vector<float> computeMeanPooling(const float* embeddings, int64_t batch_size,
                                      int64_t seq_len, int64_t embedding_dim) {
    std::vector<float> mean_embedding(embedding_dim, 0.0f);

    // Sum across sequence dimension
    for (int64_t i = 0; i < seq_len; ++i) {
        for (int64_t j = 0; j < embedding_dim; ++j) {
            mean_embedding[j] += embeddings[i * embedding_dim + j];
        }
    }

    // Divide by sequence length to get mean
    for (int64_t j = 0; j < embedding_dim; ++j) {
        mean_embedding[j] /= static_cast<float>(seq_len);
    }

    return mean_embedding;
}

// TODO: Use ONNX Runtime API
std::vector<float> l2Normalize(const std::vector<float>& vec) {
    // Compute L2 norm
    float norm = 0.0f;
    for (float val : vec) {
        norm += val * val;
    }
    norm = std::sqrt(norm);

    // Avoid division by zero
    if (norm < 1e-8f) {
        return vec;
    }

    // Normalize
    std::vector<float> normalized(vec.size());
    for (size_t i = 0; i < vec.size(); ++i) {
        normalized[i] = vec[i] / norm;
    }

    return normalized;
}

extern "C" void benchmark(char **error) {
    try {
        // Get model path
        std::string model_path = getDefaultModelPath();
        // if (argc > 1) {
        //     model_path = argv[1];
        // } else {
        //     model_path = getDefaultModelPath();
        // }

        std::cout << "Loading ONNX model from: " << model_path << std::endl;

        // Initialize ONNX Runtime environment
        Ort::Env env(ORT_LOGGING_LEVEL_WARNING, "ONNXInference");
        Ort::SessionOptions session_options;

        // Create session
        Ort::Session session(env, model_path.c_str(), session_options);

        // Get input/output info
        Ort::AllocatorWithDefaultOptions allocator;

        // Get input names and shapes
        size_t num_input_nodes = session.GetInputCount();
        std::vector<std::string> input_names;
        std::vector<std::vector<int64_t>> input_shapes;

        for (size_t i = 0; i < num_input_nodes; i++) {
            Ort::AllocatedStringPtr input_name_alloc = session.GetInputNameAllocated(i, allocator);
            std::string input_name = input_name_alloc.get();
            input_names.push_back(input_name);

            Ort::TypeInfo type_info = session.GetInputTypeInfo(i);
            auto tensor_info = type_info.GetTensorTypeAndShapeInfo();
            std::vector<int64_t> shape = tensor_info.GetShape();
            input_shapes.push_back(shape);

            std::cout << "Input " << i << ": " << input_name
                    << " shape: [";
            for (size_t j = 0; j < shape.size(); ++j) {
                std::cout << shape[j];
                if (j < shape.size() - 1) std::cout << ", ";
            }
            std::cout << "]" << std::endl;
        }

        // Get output names
        size_t num_output_nodes = session.GetOutputCount();
        std::vector<std::string> output_names;

        for (size_t i = 0; i < num_output_nodes; i++) {
            Ort::AllocatedStringPtr output_name_alloc = session.GetOutputNameAllocated(i, allocator);
            std::string output_name = output_name_alloc.get();
            output_names.push_back(output_name);

            Ort::TypeInfo type_info = session.GetOutputTypeInfo(i);
            auto tensor_info = type_info.GetTensorTypeAndShapeInfo();
            std::vector<int64_t> shape = tensor_info.GetShape();

            std::cout << "Output " << i << ": " << output_name
                    << " shape: [";
            for (size_t j = 0; j < shape.size(); ++j) {
                std::cout << shape[j];
                if (j < shape.size() - 1) std::cout << ", ";
            }
            std::cout << "]" << std::endl;
        }

        // Prepare input data
        // Input IDs
        std::vector<int64_t> input_ids = EXAMPLE_INPUT_IDS;
        std::vector<int64_t> input_ids_shape = {BATCH_SIZE, SEQUENCE_LENGTH};

        // Token type IDs (all zeros)
        std::vector<int64_t> token_type_ids(SEQUENCE_LENGTH, 0);
        std::vector<int64_t> token_type_ids_shape = {BATCH_SIZE, SEQUENCE_LENGTH};

        // Attention mask (all ones)
        std::vector<int64_t> attention_mask(SEQUENCE_LENGTH, 1);
        std::vector<int64_t> attention_mask_shape = {BATCH_SIZE, SEQUENCE_LENGTH};

        // Create input tensors
        std::vector<Ort::Value> input_tensors;

        // Find input indices by name
        int input_ids_idx = -1, token_type_ids_idx = -1, attention_mask_idx = -1;
        for (size_t i = 0; i < input_names.size(); ++i) {
            if (input_names[i] == "input_ids") {
                input_ids_idx = i;
            } else if (input_names[i] == "token_type_ids") {
                token_type_ids_idx = i;
            } else if (input_names[i] == "attention_mask") {
                attention_mask_idx = i;
            }
        }

        // Create memory info
        Ort::MemoryInfo memory_info("Cpu", OrtDeviceAllocator, 0, OrtMemTypeDefault);

        // Create vector of const char* for input names (required by Run API)
        std::vector<const char*> input_names_cstr;
        for (const auto& name : input_names) {
            input_names_cstr.push_back(name.c_str());
        }

        // Create input tensors in the correct order (matching input_names order)
        std::vector<Ort::Value> ordered_inputs;
        ordered_inputs.reserve(num_input_nodes);

        // Create tensors in the order they appear in the model
        for (size_t i = 0; i < num_input_nodes; ++i) {
            if (input_names[i] == "input_ids") {
                ordered_inputs.push_back(Ort::Value::CreateTensor<int64_t>(
                    memory_info, input_ids.data(), input_ids.size(),
                    input_ids_shape.data(), input_ids_shape.size()));
            } else if (input_names[i] == "token_type_ids") {
                ordered_inputs.push_back(Ort::Value::CreateTensor<int64_t>(
                    memory_info, token_type_ids.data(), token_type_ids.size(),
                    token_type_ids_shape.data(), token_type_ids_shape.size()));
            } else if (input_names[i] == "attention_mask") {
                ordered_inputs.push_back(Ort::Value::CreateTensor<int64_t>(
                    memory_info, attention_mask.data(), attention_mask.size(),
                    attention_mask_shape.data(), attention_mask_shape.size()));
            } else {
                std::cerr << "Warning: Unknown input name: " << input_names[i] << std::endl;
                // Create empty tensor as placeholder (should not happen with this model)
                std::vector<int64_t> empty_data(SEQUENCE_LENGTH, 0);
                std::vector<int64_t> empty_shape = {BATCH_SIZE, SEQUENCE_LENGTH};
                ordered_inputs.push_back(Ort::Value::CreateTensor<int64_t>(
                    memory_info, empty_data.data(), empty_data.size(),
                    empty_shape.data(), empty_shape.size()));
            }
        }

        // Create vector of const char* for output names (required by Run API)
        std::vector<const char*> output_names_cstr;
        for (const auto& name : output_names) {
            output_names_cstr.push_back(name.c_str());
        }

        // Run inference
        std::cout << "\nRunning inference..." << std::endl;
        Ort::RunOptions run_options;
        auto output_tensors = session.Run(run_options,
                                        input_names_cstr.data(), ordered_inputs.data(), input_names_cstr.size(),
                                        output_names_cstr.data(), output_names_cstr.size());



        // Get output tensor
        auto& output_tensor = output_tensors[0];
        float* output_data = output_tensor.GetTensorMutableData<float>();

        // Get output shape
        auto output_shape = output_tensor.GetTensorTypeAndShapeInfo().GetShape();
        int64_t batch_size_out = output_shape[0];
        int64_t seq_len_out = output_shape[1];
        int64_t embedding_dim = output_shape[2];

        std::cout << "Output shape: [" << batch_size_out << ", "
                << seq_len_out << ", " << embedding_dim << "]" << std::endl;

        // Compute mean pooling
        std::vector<float> mean_embedding = computeMeanPooling(
            output_data, batch_size_out, seq_len_out, embedding_dim);

        // L2 normalize
        std::vector<float> normalized_embedding = l2Normalize(mean_embedding);

        // Print normalized embedding
        std::cout << "\nL2-normalized mean embedding (" << normalized_embedding.size() << " dimensions):" << std::endl;
        std::cout << "[";
        for (size_t i = 0; i < normalized_embedding.size(); ++i) {
            std::cout << normalized_embedding[i];
            if (i < normalized_embedding.size() - 1) {
                std::cout << ", ";
            }
            if ((i + 1) % 10 == 0 && i < normalized_embedding.size() - 1) {
                std::cout << "\n ";
            }
        }
        std::cout << "]" << std::endl;

        // Print the mean
        float mean = 0.0f;
        for (float val : normalized_embedding) {
            mean += val;
        }
        mean /= normalized_embedding.size();
        std::cout << "Mean: " << mean << std::endl;

        // Verify L2 norm is approximately 1.0
        float norm_check = 0.0f;
        for (float val : normalized_embedding) {
            norm_check += val * val;
        }
        std::cout << "\nL2 norm verification: " << std::sqrt(norm_check) << std::endl;

        // --- Benchmark ---
        std::cout << "\nDoing benchmark for 20 seconds..." << std::endl;

        size_t num_calls = 0;
        size_t num_tokens = 0;
        size_t num_bytes = 0;
        auto start_time = std::chrono::high_resolution_clock::now();
        while (std::chrono::high_resolution_clock::now() - start_time < std::chrono::seconds(20)) {
            session.Run(run_options,
                        input_names_cstr.data(), ordered_inputs.data(), input_names_cstr.size(),
                        output_names_cstr.data(), output_names_cstr.size());

            // Get the output tensor after inference (`session.Run` does not return value directly, but updates output OrtValue)
            // We assume output tensor is ordered_outputs[0] (created before the benchmark loop)
            auto& output_tensor = output_tensors[0];
            float* output_data = output_tensor.GetTensorMutableData<float>();

            // Determine embedding dims using output shape, e.g., [batch, seq, dim] or [batch, dim]
            // We'll fetch element count and shape
            Ort::TypeInfo output_type_info = output_tensor.GetTypeInfo();
            auto output_tensor_info = output_type_info.GetTensorTypeAndShapeInfo();
            std::vector<int64_t> output_shape = output_tensor_info.GetShape();

            int64_t batch = output_shape[0];
            int64_t seq = output_shape.size() == 3 ? output_shape[1] : 1;
            int64_t emb_dim = output_shape.back();

            // Pooling: typically mean pooling over the sequence dimension
            std::vector<float> mean_embedding = computeMeanPooling(output_data, batch, seq, emb_dim);

            // L2-normalize the embedding
            // Compute the normalized embedding and use its first element in a way that cannot be optimized away
            volatile float dummy_value = l2Normalize(mean_embedding)[0];
            (void)dummy_value; // Prevent unused variable warning

            num_calls++;
        }
        num_tokens = num_calls * input_ids.size();
        num_bytes = num_calls * 384 * sizeof(float);

        auto unit_repr = [](double value) -> std::string {
            char buf[32];
            if (value > 1'000'000'000.0) {
                std::snprintf(buf, sizeof(buf), "%.1fG", value / 1'000'000'000.0);
                return std::string(buf);
            } else if (value > 1'000'000.0) {
                std::snprintf(buf, sizeof(buf), "%.1fM", value / 1'000'000.0);
                return std::string(buf);
            } else if (value > 1'000.0) {
                std::snprintf(buf, sizeof(buf), "%.1fK", value / 1'000.0);
                return std::string(buf);
            } else {
                std::snprintf(buf, sizeof(buf), "%.1f", value);
                return std::string(buf);
            }
        };

        auto end_time = std::chrono::high_resolution_clock::now();
        auto duration = std::chrono::duration_cast<std::chrono::milliseconds>(end_time - start_time);
        std::cout << "- Total time: " << (double)duration.count() / 1000.0 << "s" << std::endl;
        std::cout << "- Number of inference calls: " << unit_repr((double)num_calls / (double)duration.count() * 1000.0) << " call/s" << std::endl;
        std::cout << "- Number of input tokens: " << unit_repr((double)num_tokens / (double)duration.count() * 1000.0) << " token/s" << std::endl;
        std::cout << "- Number of output embedding bytes: " << unit_repr((double)num_bytes / (double)duration.count() * 1000.0) << " byte/s" << std::endl;
    } catch (const std::exception& e) {
        std::cerr << "Error: " << e.what() << std::endl;
        *error = strdup(e.what());
    }
}

