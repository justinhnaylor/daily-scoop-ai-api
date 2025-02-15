from google import genai
from google.genai import types
from PIL import Image
from io import BytesIO
import sys
import json
import os
import base64

def generate_image(prompt, output_path):
    try:
        # Initialize the client with API key
        api_key = os.getenv('IMAGEN_API_KEY')
        if not api_key:
            raise ValueError("IMAGEN_API_KEY environment variable not set")
            
        client = genai.Client(api_key=api_key)
        
        # Structure the prompt for better results
        formatted_prompt = f"""Create a photorealistic news article image:
{prompt}
The image should be high-quality, professional, and suitable for a news website."""

        # Generate the image
        response = client.models.generate_images(
            model='imagen-3.0-generate-002',
            prompt=formatted_prompt,
            config=types.GenerateImagesConfig(
                number_of_images=1,
                person_generation = "ALLOW_ADULT",
                aspect_ratio="16:9"  # Widescreen format for news articles
            )
        )
        
        # Save the first generated image
        if response.generated_images:
            image = Image.open(BytesIO(response.generated_images[0].image.image_bytes))
            
            # Ensure the output directory exists
            os.makedirs(os.path.dirname(output_path), exist_ok=True)
            
            # Save the image
            image.save(output_path)
            print(json.dumps({"success": True, "path": output_path}))
            return True
        
        print(json.dumps({"error": "No images generated"}), file=sys.stderr)
        return False

    except Exception as e:
        print(json.dumps({"error": str(e)}), file=sys.stderr)
        return False

if __name__ == "__main__":
    if len(sys.argv) != 3:
        print(json.dumps({"error": "Usage: python imagen_generator.py <prompt> <output_path>"}), file=sys.stderr)
        sys.exit(1)
    
    prompt = sys.argv[1]
    output_path = sys.argv[2]
    generate_image(prompt, output_path)