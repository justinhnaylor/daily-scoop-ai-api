import sys
import json
import warnings
import subprocess
import traceback
import os
import time
import threading

# Update these constants at the top of the file
MAX_TOKENS_PER_CHUNK = 150  # Reduced from 200
MAX_CHUNKS = 8  # Reduced from 12
MAX_TEXT_LENGTH = 30000  # Reduced from 50000
MAX_CONCURRENT_PROCESSES = 2  # New constant for process limiting

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
process_semaphore = threading.BoundedSemaphore(MAX_CONCURRENT_PROCESSES)  # Add semaphore
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
                    device = "mps" if torch.backends.mps.is_available() else "cpu"
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

                    # Add memory optimization settings
                    model = AutoModelForSeq2SeqLM.from_pretrained(
                        model_name,
                        config=config,
                        local_files_only=False,
                        trust_remote_code=True,
                        cache_dir=cache_dir,
                        torch_dtype=torch_dtype,
                        load_in_8bit=False,
                        max_memory={0: "1GB"},  
                        low_cpu_mem_usage=True  
                    )
                    
                    # Clear CUDA cache if available
                    if torch.cuda.is_available():
                        torch.cuda.empty_cache()
                    
                    tokenizer = AutoTokenizer.from_pretrained(
                        model_name,
                        local_files_only=False,
                        trust_remote_code=True,
                        cache_dir=cache_dir,
                    )


                    summarizer = pipeline("summarization",
                                        model=model,
                                        tokenizer=tokenizer,
                                        device=device)
                    print(json.dumps({"debug": f"Model loaded with dtype: {model.dtype if hasattr(model, 'dtype') else 'unknown'}"}), file=sys.stderr) # Debug log for dtype

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

        # Add maximum text length limit
        if len(text) > MAX_TEXT_LENGTH:
            text = text[:MAX_TEXT_LENGTH]
            print(json.dumps({"warning": f"Text truncated to {MAX_TEXT_LENGTH} characters"}), file=sys.stderr)

        # Clean the input text
        text = text.strip()
        if not text:
            raise ValueError("Empty text provided")

        # Split by paragraphs first
        paragraphs = text.split('\n\n')
        
        # Take only the most relevant paragraphs (beginning and end)
        if len(paragraphs) > MAX_CHUNKS * 2:
            paragraphs = paragraphs[:MAX_CHUNKS] + paragraphs[-2:]
        
        chunks = []
        current_chunk = []
        current_length = 0

        for paragraph in paragraphs:
            if len(chunks) >= MAX_CHUNKS:
                break

            # Split into sentences
            sentences = paragraph.replace("? ", "?\n").replace("! ", "!\n").replace(". ", ".\n").split("\n")
            
            for sentence in sentences:
                if not sentence.strip():
                    continue

                tokens = tokenizer(sentence, return_tensors="pt", truncation=True, max_length=512)
                sentence_length = len(tokens['input_ids'][0])

                if current_length + sentence_length > MAX_TOKENS_PER_CHUNK and current_chunk:
                    chunks.append(" ".join(current_chunk))
                    current_chunk = []
                    current_length = 0
                    
                    if len(chunks) >= MAX_CHUNKS:
                        break

                current_chunk.append(sentence)
                current_length += sentence_length

        if current_chunk and len(chunks) < MAX_CHUNKS:
            chunks.append(" ".join(current_chunk))

        return chunks

    except Exception as e:
        print(json.dumps({"error": f"Error in split_text_into_chunks: {str(e)}"}), file=sys.stderr)
        raise

def calculate_summary_length(text_length):
    # Aim for a summary that's about 30-40% of the original length
    target_length = min(text_length - 1, max(30, min(250, int(text_length * 0.35))))
    min_length = max(20, int(target_length * 0.6))
    return target_length, min_length

def process_chunk(chunk):
    max_retries = 1  # Reduced from 3
    retry_delay = 3  # Reduced from 5
    
    for attempt in range(max_retries):
        try:
            if not chunk.strip():
                return ""

            # More aggressive length reduction
            max_length = min(int(len(chunk.split()) * 0.3), 100)  # Reduced from 150
            min_length = max(20, int(max_length * 0.5))

            with model_lock:
                summary = summarizer(chunk,
                                  max_length=max_length,
                                  min_length=min_length,
                                  do_sample=False,
                                  num_beams=1,
                                  length_penalty=1.0,
                                  early_stopping=True)

            if not summary or not summary[0].get('summary_text'):
                raise ValueError("Empty summary generated")

            return summary[0]['summary_text']
            
        except Exception as e:
            if attempt == max_retries - 1:
                print(json.dumps({"warning": f"Skipping chunk after failure: {str(e)}"}), file=sys.stderr)
                return None
            time.sleep(retry_delay)
            continue

def summarize_text(text):
    try:
        with process_semaphore:  # Acquire semaphore before processing
            # Add maximum text length limit
            if len(text) > 50000:
                return {"success": False, "error": "Text too long for summarization (max 50KB)"}

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

            # Process chunks with longer timeout
            summaries = []
            failed_chunks = 0
            with ThreadPoolExecutor(max_workers=1) as executor:
                futures = [executor.submit(process_chunk, chunk) for chunk in chunks]
                for i, future in enumerate(futures):
                    try:
                        summary = future.result(timeout=300)  
                        if summary is None:  # Chunk processing failed
                            failed_chunks += 1
                            if failed_chunks >= 2:  # If 2 or more chunks fail, abandon the summary
                                return {"success": False, "error": "Too many chunks failed to process"}
                            continue
                        if summary:  # Only append non-empty summaries
                            summaries.append(summary)
                        # Clear memory after each chunk
                        if torch.cuda.is_available():
                            torch.cuda.empty_cache()
                    except TimeoutError:
                        failed_chunks += 1
                        if failed_chunks >= 2:  # If 2 or more chunks fail, abandon the summary
                            return {"success": False, "error": "Too many chunks timed out"}
                        print(json.dumps({"warning": f"Skipping chunk {i} due to timeout"}), file=sys.stderr)
                        continue 

            if not summaries:
                return {"success": False, "error": "No valid summaries generated"}

            final_summary = " ".join(summaries)
            if not final_summary.strip():
                return {"success": False, "error": "Generated summary is empty"}

            return {"success": True, "summary": final_summary}

    except Exception as e:
        error_msg = f"Error in summarize_text: {str(e)}"
        print(json.dumps({"error": error_msg}), file=sys.stderr)
        return {"success": False, "error": str(e)}
    finally:
        # The semaphore is automatically released when the with block exits
        pass

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