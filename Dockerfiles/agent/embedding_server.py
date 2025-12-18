#!/opt/datadog-agent/embedded/bin/python3
"""
PyTorch-based embedding server for Datadog Agent drift detection.
Provides Ollama-compatible API at http://localhost:11434/api/embed
"""

import argparse
import json
import logging
import signal
import sys
from http.server import HTTPServer, BaseHTTPRequestHandler
from typing import List

from sentence_transformers import SentenceTransformer

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s [%(levelname)s] %(message)s'
)
logger = logging.getLogger(__name__)

model = None
shutdown_flag = False


def load_model(model_name: str):
    """Load sentence-transformers model."""
    global model

    logger.info(f"Loading model: {model_name}")
    model = SentenceTransformer(model_name)
    embed_dim = model.get_sentence_embedding_dimension()
    logger.info(f"Model loaded successfully (embedding dim: {embed_dim})")


def generate_embeddings(texts: List[str]) -> List[List[float]]:
    """Generate embeddings for texts using sentence-transformers."""
    if not texts:
        return []

    # Encode texts to embeddings (already normalized by default)
    embeddings = model.encode(
        texts,
        convert_to_numpy=True,
        normalize_embeddings=True,  # L2 normalization
        show_progress_bar=False
    )

    return embeddings.tolist()


class EmbeddingHandler(BaseHTTPRequestHandler):
    def log_message(self, format, *args):
        logger.info(f"{self.address_string()} - {format % args}")

    def do_POST(self):
        if self.path != "/api/embed":
            self.send_error(404, "Not Found")
            return

        try:
            content_length = int(self.headers.get('Content-Length', 0))
            body = self.rfile.read(content_length)
            request_data = json.loads(body.decode('utf-8'))

            input_texts = request_data.get("input", [])
            model_name = request_data.get("model", "all-MiniLM-L6-v2")

            if not isinstance(input_texts, list):
                self.send_error(400, "Input must be an array of strings")
                return

            embeddings = generate_embeddings(input_texts)
            response = {"embeddings": embeddings, "model": model_name}

            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(response).encode('utf-8'))

        except Exception as e:
            logger.error(f"Error: {e}", exc_info=True)
            self.send_error(500, f"Internal error: {str(e)}")

    def do_GET(self):
        if self.path in ["/health", "/"]:
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            response = {"status": "healthy", "model": "all-MiniLM-L6-v2"}
            self.wfile.write(json.dumps(response).encode('utf-8'))
        else:
            self.send_error(404)


def signal_handler(signum, frame):
    global shutdown_flag
    logger.info(f"Received signal {signum}, shutting down...")
    shutdown_flag = True


def check_drift_detection_enabled(config_path: str) -> bool:
    """Check if drift detection is enabled in datadog.yaml."""
    try:
        import yaml
        with open(config_path, 'r') as f:
            config = yaml.safe_load(f) or {}
        drift_config = config.get('logs_config', {}).get('drift_detection', {})
        return bool(drift_config.get('enabled', False))
    except Exception as e:
        logger.warning(f"Could not read config: {e}")
        return False


def main():
    parser = argparse.ArgumentParser(description="PyTorch Embedding Server")
    default_model = "sentence-transformers/all-MiniLM-L6-v2"
    parser.add_argument("--model-name", default=default_model)
    parser.add_argument("--host", default="127.0.0.1")
    parser.add_argument("--port", type=int, default=11434)
    parser.add_argument("--config", default="/etc/datadog-agent/datadog.yaml")
    parser.add_argument("--check-config", action="store_true")

    args = parser.parse_args()

    if args.check_config and not check_drift_detection_enabled(args.config):
        logger.info("Drift detection not enabled. Exiting.")
        sys.exit(0)

    signal.signal(signal.SIGTERM, signal_handler)
    signal.signal(signal.SIGINT, signal_handler)

    try:
        load_model(args.model_name)
        server = HTTPServer((args.host, args.port), EmbeddingHandler)
        logger.info(f"Embedding server listening on {args.host}:{args.port}")

        while not shutdown_flag:
            server.handle_request()

        logger.info("Server shutdown complete")
    except Exception as e:
        logger.error(f"Fatal error: {e}", exc_info=True)
        sys.exit(1)


if __name__ == "__main__":
    main()
