import os
os.environ['PYTORCH_ENABLE_MPS_FALLBACK'] = '1'
os.environ['TRANSFORMERS_NO_ADVISORY_WARNINGS'] = '1'

from google import genai
from google.genai import types
from PIL import Image
import sys
import json
import warnings
warnings.filterwarnings("ignore")

def generate_image(prompt, output_path):
    try:
        # Initialize the API with key
        api_key = os.getenv('IMAGEN_API_KEY')
        if not api_key:
            print(json.dumps({"error": "IMAGEN_API_KEY not found in environment"}), file=sys.stderr)
            return False
            
        print(json.dumps({"debug": "API key configured"}), file=sys.stderr)
        
        # Create client with only API key
        client = genai.Client(api_key=api_key)
        
        # Structure the prompt for better results
        formatted_prompt = f"""Create a photorealistic news article image:
{prompt}
The image should be high-quality, professional, and suitable for a news website."""

        print(json.dumps({"debug": "Calling Imagen API"}), file=sys.stderr)
        try:
            # Generate the image using the client
            response = client.models.generate_images(
                model='imagen-3.0-generate-002',
                prompt=formatted_prompt,
                config=types.GenerateImagesConfig(
                    number_of_images=1,
                    aspect_ratio="16:9",
                    safety_filter_level="BLOCK_LOW_AND_ABOVE",
                    person_generation="ALLOW_ADULT"
                )
            )
            
            if response.generated_images:
                # Get the directory path
                output_dir = os.path.dirname(output_path)
                
                # Create directory if it doesn't exist and path is not empty
                if output_dir:
                    os.makedirs(output_dir, exist_ok=True)
                
                # Get the PIL image and save it
                image = response.generated_images[0].image._pil_image
                image.save(output_path)
                
                print(json.dumps({"success": True, "path": output_path}))
                return True
            
            print(json.dumps({"error": "No images generated in response"}), file=sys.stderr)
            return False
            
        except Exception as api_error:
            print(json.dumps({"error": f"Image generation failed: {str(api_error)}"}), file=sys.stderr)
            return False

    except Exception as e:
        print(json.dumps({"error": f"General error: {str(e)}"}), file=sys.stderr)
        return False

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print(json.dumps({"error": "Usage: python imagen_generator.py <prompt> <output_path>"}), file=sys.stderr)
        sys.exit(1)
    
    prompt = sys.argv[1]
    output_path = sys.argv[2]
    success = generate_image(prompt, output_path)
    if not success:
        sys.exit(1)