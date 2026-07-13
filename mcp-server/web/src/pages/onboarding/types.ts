// Shared types for the onboarding wizard.

export type VectorStoreProvider = 'pgvector' | 'qdrant' | 'chroma'
export type EmbedderProvider = 'voyage' | 'tei'

/** Draft configuration that the wizard collects across all steps. */
export interface DraftConfig {
  vectorStore: VectorStoreProvider | ''
  pgvectorDSN: string
  qdrantURL: string
  qdrantAPIKey: string
  chromaURL: string
  embedder: EmbedderProvider | ''
  voyageAPIKey: string
  voyageModel: string
  voyageDim: number
  teiURL: string
}

export const defaultDraft: DraftConfig = {
  vectorStore: '',
  pgvectorDSN: 'postgresql://enowdev@localhost:5432/enowxrag',
  qdrantURL: 'http://localhost:6333',
  qdrantAPIKey: '',
  chromaURL: 'http://localhost:8000',
  embedder: '',
  voyageAPIKey: '',
  voyageModel: 'voyage-4',
  voyageDim: 1024,
  teiURL: 'http://localhost:8081',
}

export const STEPS = ['welcome', 'vector', 'embedding', 'test', 'setup', 'install', 'done'] as const
export type Step = (typeof STEPS)[number]

export const STEP_LABELS: Record<Step, string> = {
  welcome: 'Welcome',
  vector: 'Vector Store',
  embedding: 'Embedding',
  test: 'Test',
  setup: 'Local Backend',
  install: 'Install',
  done: 'Done',
}

/** Convert DraftConfig to the flat JSON format the backend expects. */
export function draftToRequest(cfg: DraftConfig): import('../../lib/api').SetupApplyRequest {
  return {
    vector_store: cfg.vectorStore,
    embedder: cfg.embedder,
    voyage_api_key: cfg.embedder === 'voyage' ? cfg.voyageAPIKey : undefined,
    voyage_model: cfg.embedder === 'voyage' ? cfg.voyageModel : undefined,
    voyage_dim: cfg.embedder === 'voyage' ? cfg.voyageDim : undefined,
    pgvector_dsn: cfg.vectorStore === 'pgvector' ? cfg.pgvectorDSN : undefined,
    qdrant_url: cfg.vectorStore === 'qdrant' ? cfg.qdrantURL : undefined,
    qdrant_api_key: cfg.vectorStore === 'qdrant' ? cfg.qdrantAPIKey : undefined,
    chroma_url: cfg.vectorStore === 'chroma' ? cfg.chromaURL : undefined,
    tei_url: cfg.embedder === 'tei' ? cfg.teiURL : undefined,
  }
}

/** True when a URL points at the local machine (needs a local Docker service). */
function isLocalURL(url: string): boolean {
  return /localhost|127\.0\.0\.1|0\.0\.0\.0|::1/.test(url)
}

/**
 * Returns the docker-compose service names that must actually run locally for
 * this config. A remote vector store (e.g. hosted Qdrant) or an API embedder
 * (Voyage) needs no container, so it is omitted. When the result is empty,
 * nothing needs Docker and the Local Backend step can be skipped.
 */
export function localServices(cfg: DraftConfig): string[] {
  const names: string[] = []
  if (cfg.vectorStore === 'pgvector' && isLocalURL(cfg.pgvectorDSN)) names.push('postgres')
  if (cfg.vectorStore === 'qdrant' && isLocalURL(cfg.qdrantURL)) names.push('qdrant')
  if (cfg.vectorStore === 'chroma' && isLocalURL(cfg.chromaURL)) names.push('chroma')
  // Voyage is a cloud API — never needs Docker. Only self-hosted TEI does.
  if (cfg.embedder === 'tei' && isLocalURL(cfg.teiURL)) names.push('tei-embedding')
  return names
}

const SERVICE_BLOCKS: Record<string, string> = {
  postgres: `  postgres:
    image: pgvector/pgvector:pg16
    ports:
      - "5432:5432"
    environment:
      POSTGRES_DB: enowxrag
      POSTGRES_USER: enowdev
    volumes:
      - pgdata:/var/lib/postgresql/data`,
  qdrant: `  qdrant:
    image: qdrant/qdrant:latest
    ports:
      - "6333:6333"
      - "6334:6334"
    volumes:
      - qdrant_data:/qdrant/storage`,
  chroma: `  chroma:
    image: chromadb/chroma:latest
    ports:
      - "8000:8000"
    volumes:
      - chroma_data:/chroma/chroma`,
  'tei-embedding': `  tei-embedding:
    image: ghcr.io/huggingface/text-embeddings-inference:cpu-1.5
    ports:
      - "8081:80"
    volumes:
      - tei_data:/data`,
}

const SERVICE_VOLUMES: Record<string, string> = {
  postgres: '  pgdata:',
  qdrant: '  qdrant_data:',
  chroma: '  chroma_data:',
  'tei-embedding': '  tei_data:',
}

/** Generate docker-compose YAML for only the services that run locally. */
export function generateDockerCompose(cfg: DraftConfig): string {
  const names = localServices(cfg)
  const services = names.map((n) => SERVICE_BLOCKS[n])
  const volumes = names.map((n) => SERVICE_VOLUMES[n])
  return `version: "3.9"\n\nservices:\n${services.join('\n\n')}\n${volumes.length > 0 ? `\nvolumes:\n${volumes.join('\n')}` : ''}`
}

/** Generate shell commands to start the local backend. */
export function generateCommands(cfg: DraftConfig): string {
  const names = localServices(cfg)
  const lines: string[] = ['# Start local backend']
  lines.push(`docker compose up -d ${names.join(' ')}`)

  if (names.includes('postgres')) {
    lines.push('')
    lines.push('# Verify pgvector extension')
    lines.push('psql -h localhost -U enowdev -d enowxrag \\')
    lines.push('  -c "CREATE EXTENSION IF NOT EXISTS vector;"')
  }

  lines.push('')
  lines.push('# Start enowx-rag server')
  lines.push('./enowx-rag --serve --addr :7777')

  return lines.join('\n')
}
