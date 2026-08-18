from fastapi import FastAPI, File, UploadFile, HTTPException
import uvicorn
import os
import asyncio

app = FastAPI()

@app.post("/api/ocr")
async def process_document(file: UploadFile = File(...)):
    if not file.filename.lower().endswith(('.pdf', '.png', '.jpg', '.jpeg')):
        raise HTTPException(status_code=400, detail="Unsupported file format")

    # Simulate some processing time
    await asyncio.sleep(2)
    
    # MOCK OCR RESPONSE
    # We provide a dummy text that looks like a real document to test Phase 4 classification
    mock_text = """
    GOVERNMENT OF INDIA
    INCOME TAX DEPARTMENT
    
    PERMANENT ACCOUNT NUMBER CARD
    
    Name
    RAHUL SHARMA
    
    Father's Name
    SURESH SHARMA
    
    Date of Birth
    15/08/1990
    
    PAN
    ABCDE1234F
    
    Signature
    """
    
    return {"text": mock_text.strip()}

if __name__ == "__main__":
    uvicorn.run(app, host="127.0.0.1", port=8000)
