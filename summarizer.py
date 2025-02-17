import sys
import json
import warnings
import subprocess
import traceback
import os
import time
import threading

def check_and_install_dependencies():
    required_packages = {
        'setuptools': 'setuptools',  # Install setuptools first
        'transformers': 'transformers',
        'torch': 'torch',
    }

    for package, pip_name in required_packages.items():
        try:
            __import__(package)
            print(json.dumps({"debug": f"Package {package} is already installed"}), file=sys.stderr)
        except ImportError:
            print(json.dumps({"debug": f"Installing {package}..."}), file=sys.stderr)
            try:
                subprocess.check_call([sys.executable, "-m", "pip", "install", pip_name, "--timeout=60"]) # Added timeout to pip install
                print(json.dumps({"debug": f"Successfully installed {package}"}), file=sys.stderr)
            except subprocess.CalledProcessError as e:
                error_msg = f"Failed to install {package}: {str(e)}"
                print(json.dumps({"error": error_msg}), file=sys.stderr)
                sys.exit(1)

    # Suggest nightly PyTorch for MPS if desired (comment for user)
    # To install nightly PyTorch with MPS (if you want to try):
    # pip install --pre torch torchvision torchaudio --index-url https://download.pytorch.org/whl/nightly/cpu

# Check dependencies before importing
check_and_install_dependencies()

# Now import the required packages
from transformers import pipeline, AutoTokenizer, AutoConfig, AutoModelForSeq2SeqLM
import torch
from concurrent.futures import ThreadPoolExecutor


warnings.filterwarnings("ignore")

# Global variables with thread lock
model_lock = threading.Lock()
summarizer = None
tokenizer = None

def initialize_summarizer():
    try:
        global summarizer, tokenizer
        with model_lock:
            if summarizer is None:
                with warnings.catch_warnings():
                    warnings.simplefilter("ignore")
                    model_name = "sshleifer/distilbart-cnn-12-6"
                    device = "mps" if torch.backends.mps.is_available() else "cpu"  # Correct MPS check
                    torch_dtype = torch.float16 if device == "mps" else torch.float32

                    # Use user's cache directory for persistence
                    cache_dir = os.path.expanduser('~/.cache/huggingface') # User cache directory
                    os.environ['HF_HOME'] = cache_dir
                    os.environ['TRANSFORMERS_CACHE'] = cache_dir

                    # Create cache directory
                    os.makedirs(cache_dir, exist_ok=True)

                    # Try to load config, clear cache if corrupted
                    try:
                        config = AutoConfig.from_pretrained(model_name,
                                                          local_files_only=False,
                                                          use_auth_token=None,
                                                          trust_remote_code=True,
                                                          cache_dir=cache_dir,
                                                          torch_dtype=torch_dtype,
                                                          force_download=True)  # Force fresh download
                    except Exception as e:
                        if "not a valid JSON file" in str(e):
                            print(json.dumps({"debug": "Clearing corrupted cache and retrying"}), file=sys.stderr)
                            import shutil
                            model_cache_dir = os.path.join(cache_dir, "models--sshleifer--distilbart-cnn-12-6")
                            if os.path.exists(model_cache_dir):
                                shutil.rmtree(model_cache_dir)
                            config = AutoConfig.from_pretrained(model_name,
                                                              local_files_only=False,
                                                              use_auth_token=None,
                                                              trust_remote_code=True,
                                                              cache_dir=cache_dir,
                                                              torch_dtype=torch_dtype)

                    # Initialize with retry logic
                    max_retries = 3
                    retry_delay = 5
                    for attempt in range(max_retries):
                        try:
                            # Load model and tokenizer with cache_dir parameter and explicit timeout
                            model = AutoModelForSeq2SeqLM.from_pretrained(
                                model_name,
                                config=config,
                                local_files_only=False,
                                trust_remote_code=True,
                                cache_dir=cache_dir,
                                torch_dtype=torch_dtype, # Use determined dtype
                                load_in_8bit=False, # Ensure not loading in 8bit if not intended
                                max_memory=None # Let PyTorch manage memory
                            )
                            tokenizer = AutoTokenizer.from_pretrained(
                                model_name,
                                local_files_only=False,
                                trust_remote_code=True,
                                cache_dir=cache_dir,
                            )

                            # Create pipeline without timeout parameter - pipeline timeout is less reliable, control in process_chunk
                            summarizer = pipeline("summarization",
                                                model=model,
                                                tokenizer=tokenizer,
                                                device=device)
                            print(json.dumps({"debug": f"Model loaded with dtype: {model.dtype if hasattr(model, 'dtype') else 'unknown'}"}), file=sys.stderr) # Debug log for dtype
                            break
                        except Exception as e:
                            if attempt == max_retries - 1:
                                raise
                            print(json.dumps({
                                "warning": f"Attempt {attempt + 1} failed, retrying in {retry_delay} seconds: {str(e)}"
                            }), file=sys.stderr)
                            time.sleep(retry_delay)
                            retry_delay *= 2  # Exponential backoff

                    print(json.dumps({"debug": "Summarizer initialized successfully"}), file=sys.stderr)
    except Exception as e:
        print(json.dumps({"error": f"Failed to initialize summarizer: {str(e)} (Simplified error message)"}), file=sys.stderr) # Simplified error message
        raise

def split_text_into_chunks(text):
    try:
        # Initialize tokenizer if needed
        global tokenizer
        if tokenizer is None:
            initialize_summarizer()

        # Clean the input text
        text = text.strip()
        if not text:
            raise ValueError("Empty text provided")

        # Target about 500 tokens per chunk (reduced from 600 to potentially avoid timeouts)
        max_tokens = 500

        # Roughly split by sentences first to avoid cutting mid-sentence
        sentences = text.replace("? ", "?\n").replace("! ", "!\n").replace(". ", ".\n").split("\n")

        chunks = []
        current_chunk = []
        current_length = 0

        for sentence in sentences:
            # Skip empty sentences
            if not sentence.strip():
                continue

            # Get token count for this sentence
            tokens = tokenizer(sentence, return_tensors="pt", truncation=False)
            sentence_length = len(tokens['input_ids'][0])

            # If adding this sentence would exceed max_tokens, start a new chunk
            if current_length + sentence_length > max_tokens and current_chunk:
                chunks.append(" ".join(current_chunk))
                current_chunk = []
                current_length = 0

            current_chunk.append(sentence)
            current_length += sentence_length

        # Add the last chunk if it exists
        if current_chunk:
            chunks.append(" ".join(current_chunk))

        if not chunks:
            raise ValueError("No valid chunks created from input text")

        return chunks

    except Exception as e:
        print(json.dumps({"error": f"Error in split_text_into_chunks: {str(e)} (Simplified error message)"}), file=sys.stderr) # Simplified error message
        raise

def calculate_summary_length(text_length):
    # Aim for a summary that's about 30-40% of the original length
    target_length = min(text_length - 1, max(30, min(250, int(text_length * 0.35))))
    min_length = max(20, int(target_length * 0.6))
    return target_length, min_length

def process_chunk(chunk):
    max_retries = 3
    retry_delay = 10

    device = "mps" if torch.backends.mps.is_available() else "cpu" # Get device for autocast

    for attempt in range(max_retries):
        try:
            global summarizer, tokenizer

            # Log start of processing with attempt number
            print(json.dumps({"debug": f"Starting process_chunk attempt {attempt + 1}/{max_retries}"}), file=sys.stderr)

            if not chunk.strip():
                print(json.dumps({"debug": "Empty chunk detected"}), file=sys.stderr)
                return ""

            # Tokenize chunk to get token length
            tokens = tokenizer(chunk, return_tensors="pt", truncation=False)
            chunk_token_length = len(tokens['input_ids'][0])
            print(json.dumps({"debug": f"Token length: {chunk_token_length}"}), file=sys.stderr) # Simplified log

            max_length, min_length = calculate_summary_length(chunk_token_length)
            print(json.dumps({"debug": f"Summary length - max: {max_length}, min: {min_length}"}), file=sys.stderr) # Simplified log

            with warnings.catch_warnings():
                warnings.simplefilter("ignore")
                with model_lock:
                    try:
                        print(json.dumps({"debug": f"Starting model inference attempt {attempt + 1}"}), file=sys.stderr)
                        with torch.autocast(device_type=device, dtype=torch.float16 if device == "mps" else torch.float32): # Mixed precision context
                            summary = summarizer(chunk,
                                               max_length=max_length,
                                               min_length=min_length,
                                               do_sample=True,
                                               temperature=0.7)  # Removed timeout parameter
                        print(json.dumps({"debug": "Model inference completed"}), file=sys.stderr) # Simplified log
                    except Exception as model_error:
                        print(json.dumps({
                            "error": f"Model inference error (attempt {attempt + 1}): {str(model_error)}" # Simplified error message
                        }), file=sys.stderr)
                        raise

            if not summary or not summary[0].get('summary_text'):
                print(json.dumps({"error": "Model returned empty summary"}), file=sys.stderr)
                raise ValueError("Empty summary generated")

            return summary[0]['summary_text']
        except Exception as e:
            if attempt == max_retries - 1:
                raise
            if isinstance(e, (TimeoutError, RuntimeError)): # Keep RuntimeError for potential GPU issues
                print(json.dumps({
                    "warning": f"Retry attempt {attempt + 1} after error: {str(e)}" # Simplified warning
                }), file=sys.stderr)
                time.sleep(retry_delay)
                retry_delay *= 2
                continue
            raise

def summarize_text(text):
    try:
        if not text or not text.strip():
            return {"success": False, "error": "Empty or invalid input text"}

        global summarizer
        initialize_summarizer()

        # Add input validation and cleaning
        text = text.strip()
        if len(text) < 50:  # Minimum length check
            return {"success": False, "error": "Text too short for summarization"}

        try:
            chunks = split_text_into_chunks(text)
            print(json.dumps({"debug": f"Split into {len(chunks)} chunks"}), file=sys.stderr) # Simplified log
        except Exception as chunk_error:
            return {"success": False, "error": f"Chunk splitting failed: {str(chunk_error)}"}

        # Process chunks in parallel with better error handling
        summaries = []
        chunk_errors = []
        with ThreadPoolExecutor(max_workers=2) as executor:
            futures = [executor.submit(process_chunk, chunk) for chunk in chunks]
            for i, future in enumerate(futures):
                try:
                    summary = future.result(timeout=90) # Increased timeout from 60 to 90 seconds for thread wait
                    if summary:
                        summaries.append(summary)
                except TimeoutError as e: # Catch TimeoutError specifically from future.result
                    error_msg = f"Chunk {i} timed out" # Simplified timeout message
                    print(json.dumps({"error": error_msg}), file=sys.stderr)
                    chunk_errors.append(error_msg)
                except Exception as e: # Catch other exceptions
                    error_msg = f"Chunk {i} error: {str(e)}" # Simplified error message
                    print(json.dumps({"error": error_msg}), file=sys.stderr)
                    chunk_errors.append(error_msg)

        if not summaries:
            error_details = "; ".join(chunk_errors) if chunk_errors else "No valid summaries generated"
            return {"success": False, "error": error_details}

        final_summary = " ".join(summaries)
        if not final_summary.strip():
            return {"success": False, "error": "Generated summary is empty"}

        return {"success": True, "summary": final_summary}

    except Exception as e:
        error_msg = f"Error in summarize_text: {str(e)}" # Simplified top-level error message
        print(json.dumps({"error": error_msg}), file=sys.stderr)
        return {"success": False, "error": str(e)}

if __name__ == "__main__":
    print(json.dumps({"debug": "Starting summarizer script"}), file=sys.stderr)

    # Start pre-warming in a separate thread immediately
    pre_warm_thread = threading.Thread(target=initialize_summarizer, daemon=True) # daemon=True makes thread exit when main thread exits
    pre_warm_thread.start()

    try:
        while True:
            try:
                length = input()
                if not length:
                    break

                input_text = sys.stdin.read(int(length))
                if not input_text:
                    break

                print(json.dumps({"debug": f"Processing text length: {len(input_text)}"}), file=sys.stderr) # Simplified log
                result = summarize_text(input_text)

                sys.stderr.flush()
                print(json.dumps(result))
                sys.stdout.flush()

            except EOFError:
                break
            except Exception as e:
                error_msg = f"Error in main loop: {str(e)}" # Simplified main loop error message
                print(json.dumps({"error": error_msg}), file=sys.stderr)
                print(json.dumps({"success": False, "error": str(e)}))
                sys.stdout.flush()

    except Exception as e:
        error_msg = f"Fatal error: {str(e)}" # Simplified fatal error message
        print(json.dumps({"error": error_msg}), file=sys.stderr)
        sys.exit(1)