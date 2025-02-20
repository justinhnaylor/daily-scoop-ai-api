import os
import sys
import json
from google import genai
from google.genai import types

def verify_claims(keyword, summaries):
    try:
        print(json.dumps({"debug": f"Starting verification for keyword: {keyword} with {len(summaries)} summaries"}), file=sys.stderr)
        
        # Initialize the API with key
        api_key = os.getenv('GEMINI_API_KEY')
        if not api_key:
            print(json.dumps({"error": "GEMINI_API_KEY not found in environment"}), file=sys.stderr)
            return False
            
        client = genai.Client(api_key=api_key)
        print(json.dumps({"debug": "Successfully initialized Gemini client"}), file=sys.stderr)
        
        # Format summaries for the prompt
        summaries_text = "\n\n".join([f"Source: {url}\nSummary: {summary}" for url, summary in summaries.items()])
        print(json.dumps({"debug": "Formatted summaries for processing"}), file=sys.stderr)
        print(json.dumps({"debug": f"Summaries to process:\n{summaries_text[:1000]}..."}), file=sys.stderr)
        
        print(json.dumps({"debug": "Sending request to Gemini API"}), file=sys.stderr)
        
        prompt = f"""Extract and verify all factual claims about "{keyword}" from these summaries:

{summaries_text}

For each claim:
1. Extract the specific factual assertion
2. Verify its accuracy using current information
3. If inaccurate, provide the correct information"""

        try:
            response = client.models.generate_content(
                model='gemini-2.0-flash',
                contents=prompt,
                config=types.GenerateContentConfig(
                    tools=[types.Tool(
                        google_search=types.GoogleSearchRetrieval(
                            dynamic_retrieval_config=types.DynamicRetrievalConfig(
                                dynamic_threshold=0.6
                            )
                        )
                    )]
                )
            )
            
            # Split the response into chunks for better logging
            response_text = response.text
            chunk_size = 1000
            chunks = [response_text[i:i + chunk_size] for i in range(0, len(response_text), chunk_size)]
            
            print(json.dumps({"debug": "Full Gemini Response:"}), file=sys.stderr)
            for i, chunk in enumerate(chunks):
                print(json.dumps({"debug": f"Part {i + 1}: {chunk}"}), file=sys.stderr)

            # Parse claims and log them individually
            claims = parse_claims_from_response(response_text)
            print(json.dumps({"debug": "Parsed Claims:"}), file=sys.stderr)
            for i, claim in enumerate(claims):
                print(json.dumps({
                    "debug": f"Claim {i + 1}:",
                    "original": claim.get("original", "N/A"),
                    "verified": claim.get("verified", False),
                    "corrected": claim.get("corrected", "N/A"),
                    "source": claim.get("source", "N/A")
                }), file=sys.stderr)

        except Exception as e:
            print(json.dumps({"error": f"Gemini API error: {str(e)}"}), file=sys.stderr)
            raise

        # Process and format the response
        result = {
            "success": True,
            "claims": claims,
            "error": None
        }
        
        print(json.dumps({"debug": f"Verification complete. Found {len(claims)} claims."}), file=sys.stderr)
        print(json.dumps(result))
        return True

    except Exception as e:
        error_result = {
            "success": False,
            "claims": [],
            "error": str(e)
        }
        print(json.dumps({"error": f"Verification failed: {str(e)}"}), file=sys.stderr)
        print(json.dumps(error_result))
        return False

def parse_claims_from_response(response_text):
    claims = []
    current_claim = {}
    
    # Split response into lines for processing
    lines = response_text.split('\n')
    
    for line in lines:
        line = line.strip()
        
        # Skip empty lines
        if not line:
            continue
            
        # Start of a new claim
        if line.startswith("**Claim:**") or line.startswith("Claim:"):
            if current_claim:
                claims.append(current_claim)
            current_claim = {"original": line.split(":", 1)[1].strip()}
            
        # Verification status
        elif "**Verification:**" in line or "Verification:" in line:
            verification_text = line.split(":", 1)[1].strip()
            current_claim["verified"] = "true" in verification_text.lower() or "correct" in verification_text.lower()
            
        # Corrected information
        elif "**Corrected:**" in line or "Correction:" in line:
            current_claim["corrected"] = line.split(":", 1)[1].strip()
            
        # Source information
        elif "**Source:**" in line or "Source:" in line:
            current_claim["source"] = line.split(":", 1)[1].strip()
    
    # Add the last claim if exists
    if current_claim:
        claims.append(current_claim)
    
    return claims

if __name__ == "__main__":
    # Read input from stdin
    input_data = json.loads(sys.stdin.read())
    verify_claims(input_data["keyword"], input_data["summaries"]) 