import os
os.environ['PYTORCH_ENABLE_MPS_FALLBACK'] = '1'
os.environ['TRANSFORMERS_NO_ADVISORY_WARNINGS'] = '1'

import google.generativeai as genai
from google.generativeai import types
from PIL import Image
import base64
from io import BytesIO
import sys
import json
import warnings
warnings.filterwarnings("ignore")

print(json.dumps({"debug": "Python path: " + str(sys.path)}), file=sys.stderr)
print(json.dumps({"debug": "Imported packages successfully"}), file=sys.stderr)

def generate_image(prompt, output_path):
    try:
        # Initialize the API with key
        api_key = os.getenv('IMAGEN_API_KEY')
        if not api_key:
            print(json.dumps({"error": "IMAGEN_API_KEY not found in environment"}), file=sys.stderr)
            return False
            
        print(json.dumps({"debug": f"Using API key starting with: {api_key[:4]}..."}), file=sys.stderr)
        genai.configure(api_key=api_key)
        
        # Structure the prompt for better results
        formatted_prompt = f"""Create a photorealistic news article image:
{prompt}
The image should be high-quality, professional, and suitable for a news website."""

        print(json.dumps({"debug": "Calling Imagen API"}), file=sys.stderr)
        try:
            # Generate the image using the correct API format
            model = genai.GenerativeModel('imagen-3.0-generate-002')
            response = model.generate_images(
                prompt=formatted_prompt,
                number_of_images=1
            )
            print(json.dumps({"debug": f"Response received: {response}"}), file=sys.stderr)
            
        except Exception as api_error:
            print(json.dumps({"error": f"Image generation failed: {str(api_error)}"}), file=sys.stderr)
            return False

        # Save the generated image
        if response and hasattr(response, 'generated_images') and response.generated_images:
            try:
                # Get the image data from the response
                image_data = response.generated_images[0].image.image_bytes
                if not image_data:
                    print(json.dumps({"error": "Image data is empty"}), file=sys.stderr)
                    return False
                    
                print(json.dumps({"debug": f"Image data length: {len(image_data)}"}), file=sys.stderr)
                
                # Ensure the output directory exists
                os.makedirs(os.path.dirname(output_path), exist_ok=True)
                
                # Save the image
                with open(output_path, 'wb') as f:
                    f.write(image_data)
                
                print(json.dumps({"success": True, "path": output_path}))
                return True
                
            except Exception as save_error:
                print(json.dumps({"error": f"Failed to save image: {str(save_error)}"}), file=sys.stderr)
                return False
        else:
            print(json.dumps({"error": f"No image data in response. Response structure: {dir(response)}"}), file=sys.stderr)
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