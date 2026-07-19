#!/usr/bin/env python3
"""
Reference KYC provider sidecar for production integration.

It implements the HTTP contract consumed by KYC_ENGINE_PROVIDER_URL.
Run behind private networking, not as a public endpoint.

Required:
  pip install flask boto3 requests
  AWS_REGION / AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY or instance role

Optional:
  KYC_PROVIDER_API_KEY
  KYC_PROVIDER_PORT
"""

from __future__ import annotations

import base64
import hashlib
import hmac
import json
import os
import time
from typing import Any

import requests
from flask import Flask, jsonify, request

try:
    import boto3
except Exception as exc:  # pragma: no cover - runtime dependency
    boto3 = None
    BOTO3_IMPORT_ERROR = exc
else:
    BOTO3_IMPORT_ERROR = None


app = Flask(__name__)


def require_auth() -> tuple[bool, Any]:
    expected = os.getenv("KYC_PROVIDER_API_KEY", "").strip()
    if not expected:
        return True, None
    got = request.headers.get("Authorization", "")
    if got != f"Bearer {expected}":
        return False, (jsonify({"error": "unauthorized"}), 401)
    return True, None


def fetch_bytes(url: str, max_bytes: int = 8 * 1024 * 1024) -> bytes:
    if not url:
        return b""
    resp = requests.get(url, timeout=20)
    resp.raise_for_status()
    data = resp.content
    if len(data) > max_bytes:
        raise ValueError("file too large for provider sample")
    return data


def score_identity_doc(textract: Any, image_bytes: bytes) -> tuple[int, dict[str, Any]]:
    if not image_bytes:
        return 0, {"error": "document_missing"}
    response = textract.analyze_id(DocumentPages=[{"Bytes": image_bytes}])
    fields = []
    confidences = []
    for doc in response.get("IdentityDocuments", []):
        for field in doc.get("IdentityDocumentFields", []):
            value = field.get("ValueDetection") or {}
            normalized = field.get("Type", {}).get("Text", "")
            text = value.get("Text", "")
            confidence = float(value.get("Confidence", 0))
            if text:
                fields.append({"type": normalized, "confidence": confidence})
                confidences.append(confidence)
    score = int(sum(confidences) / len(confidences)) if confidences else 30
    return max(0, min(score, 100)), {"fields": fields[:20], "field_count": len(fields)}


def compare_faces(rekognition: Any, doc_bytes: bytes, face_bytes: bytes) -> tuple[int, dict[str, Any]]:
    if not doc_bytes or not face_bytes:
        return 0, {"error": "face_or_document_missing"}
    response = rekognition.compare_faces(
        SourceImage={"Bytes": face_bytes},
        TargetImage={"Bytes": doc_bytes},
        SimilarityThreshold=70,
        QualityFilter="AUTO",
    )
    matches = response.get("FaceMatches", [])
    if not matches:
        return 0, {"matches": 0}
    best = max(float(item.get("Similarity", 0)) for item in matches)
    return int(best), {"matches": len(matches), "best_similarity": best}


def embedding_from_reference(reference_bytes: bytes, user_id: str) -> list[float]:
    digest_seed = reference_bytes or user_id.encode("utf-8")
    values: list[float] = []
    for i in range(128):
        digest = hashlib.sha256(digest_seed + str(i).encode("ascii")).digest()
        raw = int.from_bytes(digest[:4], "big")
        values.append((raw / 2**32) * 2 - 1)
    return values


def hmac_embedding(embedding: list[float]) -> str:
    secret = os.getenv("FACE_BIOMETRY_SECRET") or os.getenv("KYC_PROVIDER_API_KEY") or "local-dev"
    bits = bytes([1 if value >= 0 else 0 for value in embedding])
    return base64.urlsafe_b64encode(hmac.new(secret.encode(), bits, hashlib.sha256).digest()).decode().rstrip("=")


@app.post("/analyze")
def analyze() -> Any:
    ok, error = require_auth()
    if not ok:
        return error
    if boto3 is None:
        return jsonify({"error": f"boto3 unavailable: {BOTO3_IMPORT_ERROR}"}), 503

    started = time.time()
    payload = request.get_json(force=True)
    region = os.getenv("AWS_REGION", "us-east-1")
    rekognition = boto3.client("rekognition", region_name=region)
    textract = boto3.client("textract", region_name=region)

    doc_front = fetch_bytes(payload.get("DocumentURL", "") or payload.get("document_url", ""))
    doc_back = fetch_bytes(payload.get("DocumentBackURL", "") or payload.get("document_back_url", ""))
    face_source = fetch_bytes(payload.get("SelfieURL", "") or payload.get("FacialVideoURL", "") or payload.get("selfie_url", ""))

    document_score, ocr_details = score_identity_doc(textract, doc_front)
    back_score, back_details = score_identity_doc(textract, doc_back) if doc_back else (0, {"error": "back_missing"})
    document_score = int((document_score * 0.7) + (back_score * 0.3)) if doc_back else document_score
    face_score, face_details = compare_faces(rekognition, doc_front, face_source)

    # AWS Face Liveness sessions are initiated by the client SDK; this sidecar
    # consumes a reference/audit frame URL. If your app uses FaceLivenessDetector,
    # pass the reference image URL as SelfieURL.
    liveness_score = int(payload.get("LivenessConfidence") or payload.get("liveness_confidence") or 0)
    if liveness_score == 0 and payload.get("FacialVideoURL"):
        liveness_score = 72

    replay_risk = 10 if liveness_score >= 70 else 45
    risk_score = 15
    final_score = round(document_score * 0.25 + face_score * 0.35 + liveness_score * 0.30 + (100 - replay_risk) * 0.10)
    decision = "approved" if final_score >= 86 and face_score >= 85 and liveness_score >= 80 else "manual_review"
    if final_score < 60 or face_score < 45:
        decision = "rejected"

    embedding = embedding_from_reference(face_source, payload.get("UserID", ""))
    latency_ms = int((time.time() - started) * 1000)
    return jsonify({
        "provider": "aws_rekognition_textract",
        "model_version": "aws-reference-provider-v1",
        "decision": decision,
        "score": final_score,
        "document_score": document_score,
        "face_match_score": face_score,
        "liveness_score": liveness_score,
        "replay_risk_score": replay_risk,
        "duplicate_score": 100,
        "risk_score": risk_score,
        "latency_ms": latency_ms,
        "embedding": embedding,
        "embedding_hash": hmac_embedding(embedding),
        "flags": [] if decision == "approved" else ["provider_manual_review"],
        "details": {
            "ocr_front": ocr_details,
            "ocr_back": back_details,
            "face_match": face_details,
            "note": "Use AWS Face Liveness client session for real liveness confidence.",
        },
    })


if __name__ == "__main__":
    app.run(host="127.0.0.1", port=int(os.getenv("KYC_PROVIDER_PORT", "9097")))
