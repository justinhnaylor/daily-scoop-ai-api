import sys
import json
import warnings
import subprocess
import traceback
import os
import time
import torch
from transformers import pipeline, AutoTokenizer, AutoConfig, AutoModelForSeq2SeqLM

# Suppress warnings
warnings.filterwarnings("ignore")

def check_and_install_dependencies():
    required_packages = {
        'transformers': 'transformers',
        'torch': 'torch',
    }
    for package, pip_name in required_packages.items():
        try:
            __import__(package)
            print(json.dumps({"debug": f"{package} already installed"}), file=sys.stderr)
        except ImportError:
            print(json.dumps({"debug": f"Installing {package}..."}), file=sys.stderr)
            subprocess.check_call([sys.executable, "-m", "pip", "install", pip_name, "--timeout=60"])

check_and_install_dependencies()

model = None
tokenizer = None
device = None

def initialize_summarizer():
    global model, tokenizer, device
    if model is None:
        model_name = "sshleifer/distilbart-cnn-12-6"
        cache_dir = '/tmp/.cache/huggingface'
        os.makedirs(cache_dir, exist_ok=True)
        
        # Device setup
        if torch.cuda.is_available():
            device = 0
            torch_dtype = torch.float16
        elif torch.backends.mps.is_available():
            device = torch.device('mps')
            torch_dtype = torch.float32
        else:
            device = -1
            torch_dtype = torch.float32
        
        # Load model and tokenizer
        tokenizer = AutoTokenizer.from_pretrained(model_name, cache_dir=cache_dir)
        model = AutoModelForSeq2SeqLM.from_pretrained(
            model_name,
            cache_dir=cache_dir,
            torch_dtype=torch_dtype
        ).to(device if isinstance(device, torch.device) else 'cpu')
        
        print(json.dumps({"debug": f"Model loaded on {device}"}), file=sys.stderr)

def split_text_into_chunks(text, max_tokens=500):
    tokens = tokenizer.encode(text, truncation=False, add_special_tokens=False)
    chunks = [
        tokenizer.decode(tokens[i:i+max_tokens], clean_up_tokenization_spaces=True)
        for i in range(0, len(tokens), max_tokens)
    ]
    return chunks

def summarize_text(text):
    try:
        initialize_summarizer()
        if len(text.strip()) < 50:
            return {"error": "Text too short"}
        
        chunks = split_text_into_chunks(text)
        summaries = []
        
        # Batch process all chunks
        with torch.inference_mode():
            inputs = tokenizer(
                chunks,
                max_length=500,
                truncation=True,
                padding=True,
                return_tensors="pt"
            ).to(device)
            
            outputs = model.generate(
                inputs.input_ids,
                max_length=150,
                min_length=50,
                num_beams=4,
                early_stopping=True
            )
            
            summaries = tokenizer.batch_decode(outputs, skip_special_tokens=True)
        
        return {"summary": " ".join(summaries)}
    except Exception as e:
        return {"error": str(e)}

if __name__ == "__main__":
    initialize_summarizer()
    input_text = sys.stdin.read()
    print(json.dumps(summarize_text(input_text)))