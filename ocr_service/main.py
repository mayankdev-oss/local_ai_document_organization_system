from fastapi import FastAPI, File, UploadFile, HTTPException
import uvicorn
import os
import easyocr
import fitz  # PyMuPDF
from PIL import Image
import io
import numpy as np

app = FastAPI()

# Initialize the EasyOCR reader (this runs once when the server starts)
print("Initializing EasyOCR Model (this may take a moment to load PyTorch)...")
reader = easyocr.Reader(['en'], gpu=False) # Use gpu=False for compatibility, change to True if you have CUDA installed
print("EasyOCR Model Loaded!")

@app.post("/api/ocr")
async def process_document(file: UploadFile = File(...)):
    filename = file.filename.lower()
    if not filename.endswith(('.pdf', '.png', '.jpg', '.jpeg')):
        raise HTTPException(status_code=400, detail="Unsupported file format")

    content = await file.read()
    extracted_text = []

    try:
        if filename.endswith('.pdf'):
            # Parse PDF using PyMuPDF (fitz)
            pdf_document = fitz.open(stream=content, filetype="pdf")
            for page_num in range(len(pdf_document)):
                page = pdf_document.load_page(page_num)
                # Render page to an image (pixmap)
                pix = page.get_pixmap(matrix=fitz.Matrix(2, 2)) # 2x zoom for better OCR
                # Convert pixmap to numpy array for easyocr
                img_array = np.frombuffer(pix.samples, dtype=np.uint8).reshape(pix.height, pix.width, pix.n)
                
                # If image has an alpha channel, remove it
                if pix.n == 4:
                    img_array = img_array[:, :, :3]
                
                # Read text using easyocr
                results = reader.readtext(img_array)
                for bbox, text, prob in results:
                    extracted_text.append(text)
            
            pdf_document.close()
            
        else:
            # Parse image (PNG/JPG) using Pillow and Numpy
            image = Image.open(io.BytesIO(content)).convert('RGB')
            img_array = np.array(image)
            
            # Read text using easyocr
            results = reader.readtext(img_array)
            for bbox, text, prob in results:
                extracted_text.append(text)
                
    except Exception as e:
        print(f"OCR Error: {e}")
        raise HTTPException(status_code=500, detail=f"Failed to process OCR: {str(e)}")

    final_text = "\n".join(extracted_text)
    print(f"Extracted Text:\n{final_text}")
    
    return {"text": final_text.strip()}

if __name__ == "__main__":
    uvicorn.run(app, host="127.0.0.1", port=8000)
