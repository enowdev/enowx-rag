// Shared types for the onboarding wizard.

export type VectorStoreProvider = 'pgvector' | 'qdrant' | 'chroma'
export type EmbedderProvider = 'voyage' | 'tei'
export type DeploymentMode = 'local' | 'cloud'

/** Draft configuration that the wizard collects across all steps. */
export interface DraftConfig {
  vectorStore: VectorStoreProvider | ''
  deployment: DeploymentMode
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
  deployment: 'local',
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

export const STEPS = ['welcome', 'vector', 'embedding', 'test', 'setup', 'done'] as const
export type Step = (typeof STEPS)[number]

export const STEP_LABELS: Record<Step, string> = {
  welcome: 'Welcome',
  vector: 'Vector Store',
  embedding: 'Embedding',
  test: 'Test',
  setup: 'Auto Setup',
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

/** Generate docker-compose YAML content based on draft config. */
export function generateDockerCompose(cfg: DraftConfig): string {
  const services: string[] = []

  if (cfg.vectorStore === 'pgvector') {
    services.push(`  postgres:
    image: pgvector/pgvector:pg16
    ports:
      - "5432:5432"
    environment:
      POSTGRES_DB: enowxrag
      POSTGRES_USER: enowdev
    volumes:
      - pgdata:/var/lib/postgresql/data`)
  }

  if (cfg.vectorStore === 'qdrant') {
    services.push(`  qdrant:
    image: qdrant/qdrant:latest
    ports:
      - "6333:6333"
      - "6334:6334"
    volumes:
      - qdrant_data:/qdrant/storage`)
  }

  if (cfg.vectorStore === 'chroma') {
    services.push(`  chroma:
    image: chromadb/chroma:latest
    ports:
      - "8000:8000"
    volumes:
      - chroma_data:/chroma/chroma`)
  }

  if (cfg.embedder === 'tei') {
    services.push(`  tei-embedding:
    image: ghcr.io/huggingface/text-embeddings-inference:cpu-1.5
    ports:
      - "8081:80"
    volumes:
      - tei_data:/data`)
  }

  const volumes: string[] = []
  if (cfg.vectorStore === 'pgvector') volumes.push('  pgdata:')
  if (cfg.vectorStore === 'qdrant') volumes.push('  qdrant_data:')
  if (cfg.vectorStore === 'chroma') volumes.push('  chroma_data:')
  if (cfg.embedder === 'tei') volumes.push('  tei_data:')

  return `version: "3.9"\n\nservices:\n${services.join('\n\n')}\n${volumes.length > 0 ? `\nvolumes:\n${volumes.join('\n')}` : ''}`
}

/** Generate shell commands to start the local backend. */
export function generateCommands(cfg: DraftConfig): string {
  const lines: string[] = ['# Start local backend']
  const serviceNames: string[] = []

  if (cfg.vectorStore === 'pgvector') serviceNames.push('postgres')
  if (cfg.vectorStore === 'qdrant') serviceNames.push('qdrant')
  if (cfg.vectorStore === 'chroma') serviceNames.push('chroma')
  if (cfg.embedder === 'tei') serviceNames.push('tei-embedding')

  lines.push(`docker compose up -d ${serviceNames.join(' ')}`)

  if (cfg.vectorStore === 'pgvector') {
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
