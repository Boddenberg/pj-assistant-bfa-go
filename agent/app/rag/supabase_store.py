"""
Supabase pgvector-based vector store for RAG.

Uses Supabase's built-in pgvector extension for semantic search,
replacing ChromaDB for production-grade vector storage.

Architecture:
- Table `documents` stores text chunks + embedding vectors
- Uses Supabase RPC for similarity search (cosine distance)
- Embeddings generated locally with sentence-transformers
- Falls back to ChromaDB if Supabase is not configured
"""

from __future__ import annotations

import hashlib
import json
from pathlib import Path
from dataclasses import dataclass, field

import httpx
import structlog

from app.config import settings

logger = structlog.get_logger()


@dataclass
class Document:
    """Represents a document chunk with metadata."""
    page_content: str
    metadata: dict = field(default_factory=dict)


class SupabaseVectorStore:
    """
    Vector store backed by Supabase pgvector.

    Expects the following table and function in Supabase:
    - Table: `documents` (id, content, metadata, embedding)
    - Function: `match_documents` (RPC for similarity search)
    """

    def __init__(self, url: str, service_role_key: str, embedding_fn):
        self.url = url.rstrip("/")
        self.service_role_key = service_role_key
        self.embedding_fn = embedding_fn
        self.headers = {
            "apikey": service_role_key,
            "Authorization": f"Bearer {service_role_key}",
            "Content-Type": "application/json",
            "Prefer": "return=representation",
        }
        self._client = httpx.Client(timeout=30.0)

    def _rest_url(self, path: str) -> str:
        return f"{self.url}/rest/v1/{path}"

    def _rpc_url(self, fn_name: str) -> str:
        return f"{self.url}/rest/v1/rpc/{fn_name}"

    def add_documents(self, documents: list[Document]) -> int:
        """
        Add documents to the Supabase vector store.
        Generates embeddings and inserts into the `documents` table.
        """
        if not documents:
            return 0

        texts = [doc.page_content for doc in documents]
        embeddings = self.embedding_fn.embed_documents(texts)

        rows = []
        for doc, embedding in zip(documents, embeddings):
            content_hash = hashlib.md5(doc.page_content.encode()).hexdigest()
            rows.append({
                "id": content_hash,
                "content": doc.page_content,
                "metadata": json.dumps(doc.metadata),
                "embedding": embedding,
            })

        # Upsert in batches of 50
        batch_size = 50
        inserted = 0
        for i in range(0, len(rows), batch_size):
            batch = rows[i : i + batch_size]
            try:
                resp = self._client.post(
                    self._rest_url("documents"),
                    headers={**self.headers, "Prefer": "resolution=merge-duplicates,return=representation"},
                    json=batch,
                )
                if resp.status_code in (200, 201):
                    inserted += len(batch)
                else:
                    logger.error(
                        "supabase.insert_failed",
                        status=resp.status_code,
                        body=resp.text[:500],
                    )
            except Exception as e:
                logger.error("supabase.insert_error", error=str(e))

        logger.info("supabase.documents_added", count=inserted, total=len(documents))
        return inserted

    def similarity_search_with_relevance_scores(
        self, query: str, k: int = 3
    ) -> list[tuple[Document, float]]:
        """
        Semantic search using Supabase RPC `match_documents`.
        Returns list of (Document, similarity_score) tuples.
        """
        query_embedding = self.embedding_fn.embed_query(query)

        try:
            resp = self._client.post(
                self._rpc_url("match_documents"),
                headers=self.headers,
                json={
                    "query_embedding": query_embedding,
                    "match_threshold": 0.3,
                    "match_count": k,
                },
            )

            if resp.status_code != 200:
                logger.error(
                    "supabase.search_failed",
                    status=resp.status_code,
                    body=resp.text[:500],
                )
                return []

            results = resp.json()

            docs_with_scores = []
            for row in results:
                metadata = row.get("metadata", {})
                if isinstance(metadata, str):
                    try:
                        metadata = json.loads(metadata)
                    except json.JSONDecodeError:
                        metadata = {}

                doc = Document(
                    page_content=row["content"],
                    metadata=metadata,
                )
                score = row.get("similarity", 0.0)
                docs_with_scores.append((doc, score))

            return docs_with_scores

        except Exception as e:
            logger.error("supabase.search_error", error=str(e))
            return []

    def get_document_count(self) -> int:
        """Get total number of documents in the store."""
        try:
            resp = self._client.get(
                self._rest_url("documents?select=id"),
                headers={**self.headers, "Prefer": "count=exact"},
            )
            count = resp.headers.get("content-range", "")
            if "/" in count:
                return int(count.split("/")[1])
            return 0
        except Exception:
            return 0

    def delete_all_documents(self):
        """Delete all documents from the Supabase vector store."""
        try:
            # Delete all rows (neq filter matches everything)
            resp = self._client.delete(
                self._rest_url("documents?id=neq.____never_match____"),
                headers=self.headers,
            )
            if resp.status_code in (200, 204):
                logger.info("supabase.documents_deleted")
            else:
                # Try alternative: delete where id is not null (matches all)
                resp = self._client.delete(
                    self._rest_url("documents?id=not.is.null"),
                    headers=self.headers,
                )
                logger.info("supabase.documents_deleted", status=resp.status_code)
        except Exception as e:
            logger.error("supabase.delete_error", error=str(e))

    def get_metadata_value(self, key: str) -> str | None:
        """Get a metadata value from a special _meta document."""
        try:
            resp = self._client.get(
                self._rest_url(f"documents?id=eq._meta_{key}&select=content"),
                headers=self.headers,
            )
            if resp.status_code == 200:
                data = resp.json()
                if data:
                    return data[0].get("content")
            return None
        except Exception:
            return None

    def set_metadata_value(self, key: str, value: str):
        """Store a metadata value in a special _meta document."""
        try:
            row = {
                "id": f"_meta_{key}",
                "content": value,
                "metadata": json.dumps({"type": "meta", "key": key}),
                "embedding": [0.0] * 384,  # dummy embedding
            }
            self._client.post(
                self._rest_url("documents"),
                headers={**self.headers, "Prefer": "resolution=merge-duplicates,return=representation"},
                json=[row],
            )
        except Exception as e:
            logger.error("supabase.set_metadata_error", error=str(e))


def build_supabase_store(embedding_fn) -> SupabaseVectorStore:
    """Create and optionally seed the Supabase vector store."""
    store = SupabaseVectorStore(
        url=settings.supabase_url,
        service_role_key=settings.supabase_service_role_key,
        embedding_fn=embedding_fn,
    )

    # Compute a hash of the current knowledge base content
    kb_hash = _compute_kb_hash()
    stored_hash = store.get_metadata_value("kb_content_hash")

    if stored_hash != kb_hash:
        logger.info(
            "supabase.kb_changed_reseeding",
            old_hash=stored_hash,
            new_hash=kb_hash,
        )
        store.delete_all_documents()
        _seed_knowledge_base(store)
        store.set_metadata_value("kb_content_hash", kb_hash)
    else:
        doc_count = store.get_document_count()
        if doc_count == 0:
            logger.info("supabase.seeding_knowledge_base")
            _seed_knowledge_base(store)
            store.set_metadata_value("kb_content_hash", kb_hash)
        else:
            logger.info("supabase.kb_up_to_date", docs=doc_count)

    return store


def _compute_kb_hash() -> str:
    """Compute a SHA-256 hash of all knowledge base files to detect changes."""
    kb_path = Path(settings.knowledge_base_dir)
    if not kb_path.exists():
        return "empty"

    hasher = hashlib.sha256()
    for file_path in sorted(kb_path.glob("**/*.md")):
        hasher.update(file_path.name.encode())
        hasher.update(file_path.read_text(encoding="utf-8").encode())

    return hasher.hexdigest()[:16]


def _seed_knowledge_base(store: SupabaseVectorStore):
    """Load knowledge base files, chunk them, and insert into Supabase."""
    from langchain.text_splitter import RecursiveCharacterTextSplitter

    kb_path = Path(settings.knowledge_base_dir)
    if not kb_path.exists():
        logger.warning("supabase.knowledge_base_not_found", path=str(kb_path))
        return

    text_splitter = RecursiveCharacterTextSplitter(
        chunk_size=500,
        chunk_overlap=100,
        separators=["\n\n", "\n", ". ", " ", ""],
    )

    documents = []
    for file_path in sorted(kb_path.glob("**/*.md")):
        try:
            content = file_path.read_text(encoding="utf-8")
            chunks = text_splitter.split_text(content)
            for chunk in chunks:
                documents.append(Document(
                    page_content=chunk,
                    metadata={"source": file_path.name},
                ))
        except Exception as e:
            logger.error("supabase.file_read_error", file=str(file_path), error=str(e))

    if documents:
        store.add_documents(documents)
        logger.info("supabase.seeding_complete", chunks=len(documents))
