Write-Host "==================================" -ForegroundColor Cyan
Write-Host " Starting DocuNest Local Services " -ForegroundColor Cyan
Write-Host "==================================" -ForegroundColor Cyan

# Check if Ollama is responding
Write-Host "`n[1/3] Checking Ollama AI Service..." -ForegroundColor Yellow
try {
    $response = Invoke-WebRequest -Uri "http://localhost:11434" -UseBasicParsing -ErrorAction Stop
    Write-Host "Ollama is running!" -ForegroundColor Green
} catch {
    Write-Host "WARNING: Ollama does not seem to be running on port 11434." -ForegroundColor Red
    Write-Host "Please start the Ollama application from your Windows Start menu." -ForegroundColor Red
}

# Start Python OCR Service in a new window
Write-Host "`n[2/3] Starting Python OCR Service (Mocked)..." -ForegroundColor Yellow
Start-Process powershell -ArgumentList "-NoExit -Command `"cd ocr_service; python main.py`""
Write-Host "OCR Service launching in a new window." -ForegroundColor Green

# Start Go Backend Server in a new window
Write-Host "`n[3/3] Starting Go API Backend..." -ForegroundColor Yellow
Start-Process powershell -ArgumentList "-NoExit -Command `"go run ./cmd/api/main.go`""
Write-Host "Go Backend launching in a new window." -ForegroundColor Green

Write-Host "`n==================================" -ForegroundColor Cyan
Write-Host "All services have been launched!  " -ForegroundColor Green
Write-Host "Access the app at: http://localhost:8080" -ForegroundColor Cyan
Write-Host "==================================" -ForegroundColor Cyan
