from fastapi import FastAPI, File, UploadFile, HTTPException
import uvicorn
import io
import pymupdf as fitz  # PyMuPDF
from PIL import Image, ImageFilter
import pytesseract

app = FastAPI()

# On Windows, Tesseract is installed to this path by default via the UB Mannheim installer.
# On Linux/Docker (production), this line is not needed — tesseract is on PATH.
import platform
if platform.system() == "Windows":
    pytesseract.pytesseract.tesseract_cmd = r"C:\Program Files\Tesseract-OCR\tesseract.exe"

ALLOWED_MIME_TYPES = {"application/pdf", "image/jpeg", "image/png"}

@app.post("/api/ocr")
async def process_document(file: UploadFile = File(...)):
    filename = file.filename.lower()
    if not filename.endswith(('.pdf', '.png', '.jpg', '.jpeg')):
        raise HTTPException(status_code=400, detail="Unsupported file format")

    content = await file.read()
    extracted_text = []

    try:
        if filename.endswith('.pdf'):
            pdf_document = fitz.open(stream=content, filetype="pdf")

            for page_num in range(len(pdf_document)):
                page = pdf_document.load_page(page_num)

                # Strategy 1: try native text extraction (digital PDFs — zero cost, instant)
                native_text = page.get_text("text").strip()
                if native_text:
                    extracted_text.append(native_text)
                    continue

                # Strategy 2: page is image-only — render and run Tesseract
                # 300 DPI equivalent: scale factor 4 gives ~288 DPI from 72 DPI base
                pix = page.get_pixmap(matrix=fitz.Matrix(3, 3))
                img = Image.frombytes("RGB", [pix.width, pix.height], pix.samples)
                img = preprocess_image(img)
                text = pytesseract.image_to_string(img, lang="eng", config="--psm 3")
                if text.strip():
                    extracted_text.append(text.strip())

            pdf_document.close()

        else:
            # Image file (PNG/JPG)
            img = Image.open(io.BytesIO(content)).convert("RGB")
            img = preprocess_image(img)
            text = pytesseract.image_to_string(img, lang="eng", config="--psm 3")
            if text.strip():
                extracted_text.append(text.strip())

    except Exception as e:
        print(f"OCR Error: {type(e).__name__}: {e}")
        raise HTTPException(status_code=500, detail="Failed to process document. Check server logs.")

    final_text = "\n\n".join(extracted_text).strip()
    print(f"OCR complete — extracted {len(final_text)} characters")

    return {"text": final_text}


def preprocess_image(img: Image.Image) -> Image.Image:
    """Apply light preprocessing to improve OCR accuracy on scanned documents."""
    # Convert to grayscale
    img = img.convert("L")
    # Mild sharpening helps with slightly blurry scans
    img = img.filter(ImageFilter.SHARPEN)
    return img


if __name__ == "__main__":
    uvicorn.run(app, host="127.0.0.1", port=8000)
