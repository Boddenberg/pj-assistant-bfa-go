"""
RAG Retriever — semantic search over the knowledge base.

Supports two backends:
1. Supabase pgvector (production) — managed PostgreSQL with vector extension
2. ChromaDB (local fallback) — simple, no infra dependency

Strategy:
- Embeddings: sentence-transformers (all-MiniLM-L6-v2) — fast, multilingual-capable
- Chunking: RecursiveCharacterTextSplitter — respects paragraph boundaries
- Retrieval: top-k similarity search with score threshold to avoid irrelevant context
"""

from __future__ import annotations

from pathlib import Path
from typing import Any

import structlog
from langchain.text_splitter import RecursiveCharacterTextSplitter
from langchain_community.document_loaders import DirectoryLoader, TextLoader
from langchain_community.embeddings import HuggingFaceEmbeddings
from langchain_community.vectorstores import Chroma

from app.config import settings

logger = structlog.get_logger()

# Module-level singleton for the vector store
_vector_store: Any | None = None
_using_supabase: bool = False


def _get_embedding_fn() -> HuggingFaceEmbeddings:
    """Create the embedding function (shared between backends)."""
    return HuggingFaceEmbeddings(
        model_name=settings.embedding_model,
        model_kwargs={"device": "cpu"},
        encode_kwargs={"normalize_embeddings": True},
    )


def get_vector_store():
    """Get or initialize the vector store singleton (Supabase or ChromaDB)."""
    global _vector_store, _using_supabase

    if _vector_store is not None:
        return _vector_store

    embedding_fn = _get_embedding_fn()

    # Try Supabase first if configured
    if settings.use_supabase and settings.supabase_url and settings.supabase_service_role_key:
        try:
            from app.rag.supabase_store import build_supabase_store

            logger.info("rag.using_supabase_pgvector", url=settings.supabase_url)
            _vector_store = build_supabase_store(embedding_fn)
            _using_supabase = True
            return _vector_store
        except Exception as e:
            logger.error("rag.supabase_init_failed", error=str(e))
            logger.info("rag.falling_back_to_chromadb")

    # Fallback to ChromaDB
    logger.info("rag.using_chromadb")
    persist_dir = Path(settings.chroma_persist_dir)

    if persist_dir.exists() and any(persist_dir.iterdir()):
        logger.info("rag.loading_existing_store", path=str(persist_dir))
        _vector_store = Chroma(
            persist_directory=str(persist_dir),
            embedding_function=embedding_fn,
        )
        return _vector_store

    logger.info("rag.building_store", knowledge_base=settings.knowledge_base_dir)
    _vector_store = _build_chroma_store(embedding_fn, persist_dir)
    return _vector_store


def _build_chroma_store(embedding_fn: HuggingFaceEmbeddings, persist_dir: Path) -> Chroma:
    """
    Build a ChromaDB vector store from scratch.

    Chunking strategy:
    - chunk_size=500: small enough for precise retrieval, large enough for coherent context
    - chunk_overlap=100: ensures boundary information isn't lost
    - RecursiveCharacterTextSplitter: splits on paragraph > sentence > word boundaries
    """
    kb_path = Path(settings.knowledge_base_dir)
    if not kb_path.exists():
        logger.warning("rag.knowledge_base_not_found", path=str(kb_path))
        return Chroma(
            persist_directory=str(persist_dir),
            embedding_function=embedding_fn,
        )

    loader = DirectoryLoader(
        str(kb_path),
        glob="**/*.md",
        loader_cls=TextLoader,
        loader_kwargs={"encoding": "utf-8"},
    )
    documents = loader.load()

    if not documents:
        logger.warning("rag.no_documents_found")
        return Chroma(
            persist_directory=str(persist_dir),
            embedding_function=embedding_fn,
        )

    text_splitter = RecursiveCharacterTextSplitter(
        chunk_size=500,
        chunk_overlap=100,
        separators=["\n\n", "\n", ". ", " ", ""],
        length_function=len,
    )

    chunks = text_splitter.split_documents(documents)
    logger.info("rag.chunked_documents", total_docs=len(documents), total_chunks=len(chunks))

    store = Chroma.from_documents(
        documents=chunks,
        embedding=embedding_fn,
        persist_directory=str(persist_dir),
    )

    logger.info("rag.store_built", chunks_indexed=len(chunks))
    return store


def search_knowledge_base(query: str, top_k: int = 3, score_threshold: float = 0.3):
    """
    Semantic search over the knowledge base.

    Works with both Supabase pgvector and ChromaDB backends.
    Filters out low-relevance results to avoid injecting noise into the prompt.
    """
    store = get_vector_store()

    try:
        results_with_scores = store.similarity_search_with_relevance_scores(query, k=top_k)

        # Filter by relevance threshold to avoid injecting noise
        filtered = [doc for doc, score in results_with_scores if score >= score_threshold]

        backend = "supabase" if _using_supabase else "chromadb"
        logger.info(
            "rag.search_completed",
            backend=backend,
            query_length=len(query),
            total_results=len(results_with_scores),
            filtered_results=len(filtered),
        )

        return filtered

    except Exception as e:
        logger.error("rag.search_failed", error=str(e))
        return []
