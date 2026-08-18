# DocuNest

DocuNest is a private, local-first document organization platform that automatically scans, reads, classifies, and catalogs documents for individual customers without sending any sensitive data to third-party cloud services.

Designed for legal, financial, and business workflows where document confidentiality is non-negotiable, DocuNest combines local optical character recognition (OCR), on-device artificial intelligence, and human-in-the-loop review to ensure reliable, zero-leakage records management.

---

## Table of Contents

- Overview for Non-Technical Users
- Core Capabilities
- System Architecture
- Security and Privacy Principles
- Prerequisites
- Quick Start Guide
- Detailed Manual Setup
- Workflow Walkthrough
- API Specification
- Production and Containerization Notes

---

## Overview for Non-Technical Users

### What DocuNest Does

In most offices, organizing client files requires opening dozens of scanned PDFs or photos, reading names and document numbers manually, creating folders, and renaming files one by one.

DocuNest turns this into a three-step automated routine:

1. **You drop a document into the system**: Upload a scan, photo, or PDF (such as an ID card, tax form, contract, or invoice).
2. **The system reads and understands it locally**: DocuNest extracts the printed text and asks a local AI model who the document belongs to and what type of document it is.
3. **You verify with one click**: Instead of letting artificial intelligence make uncontrolled decisions, DocuNest presents you with a quick review screen. You confirm or correct the details, and the file is cataloged under the appropriate customer folder with an in-browser preview available at any time.

### Why It Matters

- **Complete Privacy**: Everything runs on your machine or private server. No customer data, ID numbers, or financial details are ever transmitted over the internet or used to train external public models.
- **Human Oversight**: The software never merges or misfiles documents without your explicit approval.
- **Instant Search and Retrieval**: Expand any customer profile to view a clean tree of all their attached files and inspect them directly in your browser.

---

## Core Capabilities

- **Local Optical Character Recognition**: Reads scanned image files and multi-page PDFs using PyMuPDF and EasyOCR without requiring high-end graphics cards.
- **On-Device Classification**: Communicates with local language models via Ollama to intelligently determine document categories (such as Aadhaar, PAN, Passport, Invoice, Contract) and detect customer names.
- **Interactive Review Flow**: Enforces a "Needs Review" state where staff members verify AI extraction before files are committed to customer profiles.
- **Customer Document Tree and In-App Viewer**: Expandable customer profiles with integrated, secure in-browser viewing for PDFs and images.
- **Enterprise-Grade Access Controls**: Password hashing using Argon2id, cookie-based session management, and multi-tenant data isolation.
- **Tamper-Evident Audit Logging**: Every manual approval and AI extraction confirmation is recorded with timestamped metadata.

---

## System Architecture

DocuNest is designed as a decoupled, modular system with three cooperating services:

```
[ Web Browser / Alpine.js UI ]
            |
            v  (Port 8080 - HTTP with Secure Cookies)
[ Core Application Engine - Go / Gorilla Mux ]
      |                 |                   |
      v                 v                   v
[ PostgreSQL 15 ]   [ OCR Service ]    [ Local LLM / Ollama ]
 (Relational Data,   (FastAPI / Python)   (Mistral / Llama 3)
  User Isolation)     (Port 8000)          (Port 11434)
```

### Component Breakdown

1. **Frontend (public/index.html)**
   - Single-page application built with Tailwind CSS and Alpine.js.
   - Zero build step required; runs directly from Go static asset serving.
   - Manages interactive states: authentication, document uploads, review modals, customer document trees, and embedded document preview iframes.

2. **Core API Server (cmd/api/main.go & internal/)**
   - Written in Go for fast execution, low memory consumption, and reliable concurrency.
   - Manages user sessions, authentication middleware, file validation, rate limiting, and relational database operations.
   - Coordinates asynchronous document processing pipelines between OCR, Ollama, and storage.

3. **OCR Engine (ocr_service/main.py)**
   - FastAPI microservice running PyMuPDF (fitz) and EasyOCR on CPU.
   - Converts multi-page PDFs into high-resolution bitmap arrays and extracts raw text streams.

4. **Intelligence Layer (Ollama)**
   - Local model server executing open-weights language models.
   - Extracts structured JSON representing the document type, primary individual, and confidence scores from raw OCR output.

5. **Data Layer (PostgreSQL)**
   - Multi-tenant relational storage maintaining users, customers, documents, and audit logs.
   - Strict `user_id` scoping across all tables prevents data leakage between accounts.

---

## Security and Privacy Principles

DocuNest adheres to strict security and privacy standards:

- **Argon2id Cryptographic Hashing**: All authentication credentials are encrypted using the Argon2id algorithm, resistant to GPU-accelerated brute-force attacks.
- **HttpOnly and SameSite Cookies**: Session identifiers are stored exclusively in secure, browser-restricted cookies to protect against cross-site scripting (XSS) and cross-site request forgery (CSRF).
- **MIME Byte Inspection**: Uploaded files undergo binary content inspection (`http.DetectContentType`) rather than trusting file extensions alone, neutralizing malicious file uploads.
- **File System Isolation**: Uploaded files are renamed using cryptographically secure random hexadecimal UUIDs and stored outside the public document root.
- **Rate-Limiting Protection**: IP-based rate limiting on upload routes prevents disk exhaustion and denial-of-service attempts.
- **Multi-Tenant Isolation**: Queries require context-injected user identities, ensuring no authenticated user can access, query, or view files owned by another account.

---

## Prerequisites

Before running DocuNest, ensure your environment meets the following requirements:

1. **Operating System**: Windows 10/11, macOS, or Linux.
2. **Go**: Version 1.21 or newer.
3. **Python**: Version 3.10 or newer (with `pip`).
4. **PostgreSQL**: Version 14 or newer (or Docker to run PostgreSQL in a container).
5. **Ollama**: Installed and running locally with at least one model pulled (e.g., `ollama pull mistral` or `ollama pull llama3:8b`).

---

## Quick Start Guide

### Option 1: Automated Start (Windows PowerShell)

DocuNest includes an automated orchestrator script that cleans up dangling ports, verifies Ollama, and launches all microservices.

1. Clone the repository:
   ```bash
   git clone https://github.com/mayankdev-oss/local_ai_document_organization_system.git
   cd local_ai_document_organization_system
   ```

2. Start PostgreSQL:
   ```bash
   docker-compose up -d
   ```

3. Install Python dependencies:
   ```bash
   cd ocr_service
   pip install -r requirements.txt
   cd ..
   ```

4. Launch all services:
   ```powershell
   .\start.ps1
   ```

5. Open your browser and navigate to:
   ```
   http://localhost:8080
   ```
   Default credentials:
   - **Username**: `admin`
   - **Password**: `admin`

---

## Detailed Manual Setup

If you prefer to start each service individually or are deploying on a Linux/macOS server, follow these steps:

### Step 1: Database Setup

Create a `.env` file in the root directory:

```env
DB_HOST=localhost
DB_PORT=5432
DB_USER=docunest
DB_PASSWORD=password
DB_NAME=docunest
JWT_SECRET=your_super_secret_key_change_in_production
```

Start PostgreSQL via Docker or your system service:
```bash
docker-compose up -d
```

### Step 2: Configure Ollama

1. Start the Ollama background daemon:
   ```bash
   ollama serve
   ```
2. Pull your preferred model:
   ```bash
   ollama pull mistral
   ```

### Step 3: Start the Python OCR Microservice

```bash
cd ocr_service
python -m venv venv
# On Windows:
.\venv\Scripts\activate
# On Linux/macOS:
source venv/bin/activate

pip install -r requirements.txt
python main.py
```
The OCR service will listen on `http://127.0.0.1:8000`.

### Step 4: Start the Go Backend Server

In a new terminal window from the root repository directory:

```bash
go run ./cmd/api/main.go
```
The application will connect to PostgreSQL, initialize database tables, seed the default admin account, and begin serving at `http://localhost:8080`.

---

## Workflow Walkthrough

1. **Authentication**: Log in through the secure portal.
2. **Uploading**: Navigate to "Upload Document". Choose whether the document belongs to a new customer or an existing record.
3. **Background Processing**:
   - The file is securely stored on disk with a randomized identifier.
   - The Go engine posts the file to the Python OCR service.
   - PyMuPDF converts PDF pages into image buffers, and EasyOCR extracts raw text.
   - The text payload is dispatched to Ollama, which extracts the candidate's name and document category.
   - The document transitions to the `needs_review` state.
4. **Interactive Verification**:
   - Visit the "Documents" tab to see documents marked as "Needs Review".
   - Click "Review" to verify or edit the extracted name and type, and map it to the correct customer.
5. **Customer Dossier & Preview**:
   - Visit the "Customers" tab.
   - Click any customer row to expand their folder tree.
   - Click "Preview" to load the document in an in-app viewer.

---

## API Specification

All protected routes require an authenticated session cookie.

### Authentication

- `POST /api/login`
  - Body: `{"username": "admin", "password": "password"}`
  - Response: Sets `session_token` HTTP-only cookie.

- `GET /api/ping`
  - Health check and session verification endpoint.

### Customer Management

- `GET /api/customers`
  - Optional Query: `?q=search_term`
  - Response: Array of customer records scoped to authenticated user.

- `GET /api/customers/{id}/documents`
  - Response: List of all finalized documents attached to the given customer.

### Document Pipeline

- `GET /api/documents`
  - Response: List of all recent uploads, status, classification, and customer association.

- `POST /api/documents/upload`
  - Multipart form upload (`document`, `customer_type`, `customer_id`).
  - Enforces MIME validation, rate limits, and triggers asynchronous OCR and AI classification.

- `POST /api/documents/{id}/confirm`
  - Body: `{"person_name": "...", "document_type": "...", "customer_id": "..."}`
  - Commits the reviewed document and writes an entry to `audit_logs`.

- `GET /api/documents/{id}/view`
  - Streams the raw PDF or image file securely with appropriate content headers for inline viewing.

### Metrics

- `GET /api/stats`
  - Returns counts for total documents, customers, and records processed today.

---

## Production and Containerization Notes

When preparing DocuNest for production deployment on a remote server:

1. **Environment Variables**: Replace `JWT_SECRET` and database credentials in `.env` with strong random secrets.
2. **HTTPS / TLS**: Terminate TLS via Nginx, Caddy, or Cloudflare to ensure the `Secure` flag on session cookies functions properly.
3. **Containerization**:
   - The system is structured for multi-stage Docker builds.
   - The Go backend compiles into a single statically linked binary with minimal footprint.
   - The Python OCR service can run in a slim Debian container with PyTorch CPU dependencies.
   - Ollama can be hosted as an independent sidecar container with volume mounts for cached model weights.

---

## License

This project is licensed under the MIT License. You are free to use, modify, and distribute it for private or commercial applications.
