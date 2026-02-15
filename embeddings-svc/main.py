import os
import threading
from typing import List

from fastapi import FastAPI, HTTPException
from pydantic import BaseModel, Field
from sentence_transformers import SentenceTransformer


class EmbedRequest(BaseModel):
    texts: List[str] = Field(default_factory=list)


class EmbedResponse(BaseModel):
    embeddings: List[List[float]]


MODEL_PATH = os.getenv("EMBEDDINGS_MODEL_PATH", "/models/all-MiniLM-L6-v2")
MODEL_NAME = os.getenv("EMBEDDINGS_MODEL", "sentence-transformers/all-MiniLM-L6-v2")

if os.path.isdir(MODEL_PATH):
    model = SentenceTransformer(MODEL_PATH, device="cpu")
else:
    model = SentenceTransformer(MODEL_NAME, device="cpu")

model_lock = threading.Lock()

app = FastAPI(title="flux-embeddings-svc", version="1.0.0")


@app.get("/health")
def health() -> dict:
    return {"status": "ok", "model": "all-MiniLM-L6-v2", "dimensions": 384}


@app.post("/embed", response_model=EmbedResponse)
def embed(req: EmbedRequest) -> EmbedResponse:
    if not req.texts:
        return EmbedResponse(embeddings=[])

    clean_texts = [t.strip() for t in req.texts if t and t.strip()]
    if len(clean_texts) != len(req.texts):
        raise HTTPException(status_code=400, detail="texts must be non-empty strings")

    with model_lock:
        vectors = model.encode(
            clean_texts,
            batch_size=32,
            show_progress_bar=False,
            normalize_embeddings=False,
            convert_to_numpy=True,
        )

    return EmbedResponse(embeddings=vectors.tolist())
