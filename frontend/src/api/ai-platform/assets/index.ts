import request from '@/config/axios'

export interface Asset {
  id: number
  projectId: number
  filename: string
  uri: string
  thumbnailUri: string
  thumbnailUrl: string
  sha256: string
  contentType: string
  size: number
  width: number
  height: number
  mediaKind: 'IMAGE' | 'VIDEO' | 'TEXT'
  durationSeconds: number
  codec: string
  language: string
  sourceDevice: string
  businessScene: string
  perceptualHash: string
  nearDuplicateOfId: number
  status: 'READY' | 'INVALID' | 'MISSING' | 'DELETED' | 'PURGED'
  duplicateOfId: number
  errorCode: string
  errorMessage: string
  deletedAt?: string
  purgeEligibleAt?: string
  tags: Array<{
    id: number
    definitionId: number
    category: 'BUSINESS' | 'SCENE' | 'SOURCE'
    tag: string
    name: string
    color: string
  }>
  createTime: string
}

export interface AssetPage {
  list: Asset[]
  total: number
}

export interface AssetQuality {
  byStatus: Array<{ status: string; count: number }>
  duplicateGroups: number
  nearDuplicateGroups: number
}

export interface AssetDetail {
  asset: Asset
  metadata: Record<string, unknown>
  tags: Asset['tags']
  references: Array<{
    resourceType: 'ASSET_COLLECTION' | 'ANNOTATION_TASK' | 'DATASET_VERSION' | 'FEEDBACK_BATCH'
    resourceId: number
    name: string
    status: string
    frozen: boolean
  }>
  previewUrl: string
  thumbnailUrl: string
  previewExpiresIn: number
  recycleRetentionDays: number
  purgeEligibleAt?: string
}

export interface ImportRun {
  id: number
  jobId: number
  sourceType: string
  status: string
  total: number
  succeeded: number
  failed: number
}

export const getAssetPage = (projectId: number, params: Record<string, unknown>) =>
  request.get<AssetPage>({ url: `/ai-platform/projects/${projectId}/assets`, params })

export const getAssetQuality = (projectId: number) =>
  request.get<AssetQuality>({ url: `/ai-platform/projects/${projectId}/assets/quality` })

export const getAssetDetail = (projectId: number, assetId: number) =>
  request.get<AssetDetail>({ url: `/ai-platform/projects/${projectId}/assets/${assetId}` })

export const createAssetImport = (
  projectId: number,
  data: { sourceType: string; source: string; duplicatePolicy: string; options: object }
) =>
  request.post<{ importRun: ImportRun }>({
    url: `/ai-platform/projects/${projectId}/asset-imports`,
    data
  })

export const getAssetImport = (projectId: number, importId: number) =>
  request.get<{ importRun: ImportRun; report: unknown[] }>({
    url: `/ai-platform/projects/${projectId}/asset-imports/${importId}`
  })

export const recycleAsset = (projectId: number, assetId: number) =>
  request.delete({ url: `/ai-platform/projects/${projectId}/assets/${assetId}` })

export const restoreAsset = (projectId: number, assetId: number) =>
  request.post({ url: `/ai-platform/projects/${projectId}/assets/${assetId}/restore` })

export const purgeAsset = (projectId: number, assetId: number) =>
  request.post({ url: `/ai-platform/projects/${projectId}/assets/${assetId}/purge` })

const sha256 = async (blob: Blob) => {
  const digest = await crypto.subtle.digest('SHA-256', await blob.arrayBuffer())
  return Array.from(new Uint8Array(digest))
    .map((value) => value.toString(16).padStart(2, '0'))
    .join('')
}

export const uploadAsset = async (
  projectId: number,
  file: File,
  onProgress: (progress: number) => void,
  metadata: { language?: string; sourceDevice?: string; businessScene?: string } = {}
) => {
  const chunkSize = 5 * 1024 * 1024
  const resumeKey = `visionai:upload:${projectId}:${file.name}:${file.size}:${file.lastModified}`
  let session: { id: string; totalChunks: number }
  let receivedParts = new Set<number>()
  const previousSessionId = localStorage.getItem(resumeKey)
  if (previousSessionId) {
    try {
      const progress = await request.get<{
        session: {
          id: string
          totalChunks: number
          totalSize: number
          status: string
          expiresAt: string
        }
        receivedParts: number[]
      }>({ url: `/ai-platform/projects/${projectId}/uploads/${previousSessionId}` })
      if (
        progress.session.totalSize !== file.size ||
        progress.session.status === 'COMPLETED' ||
        new Date(progress.session.expiresAt).getTime() <= Date.now()
      ) {
        throw new Error('stored upload session is no longer resumable')
      }
      session = progress.session
      receivedParts = new Set(progress.receivedParts)
    } catch {
      localStorage.removeItem(resumeKey)
      session = await request.post<{ id: string; totalChunks: number }>({
        url: `/ai-platform/projects/${projectId}/uploads`,
        data: {
          filename: file.name,
          contentType: file.type,
          totalSize: file.size,
          chunkSize,
          duplicatePolicy: 'REFERENCE',
          metadata
        }
      })
      localStorage.setItem(resumeKey, session.id)
    }
  } else {
    session = await request.post<{ id: string; totalChunks: number }>({
      url: `/ai-platform/projects/${projectId}/uploads`,
      data: {
        filename: file.name,
        contentType: file.type,
        totalSize: file.size,
        chunkSize,
        duplicatePolicy: 'REFERENCE',
        metadata
      }
    })
    localStorage.setItem(resumeKey, session.id)
  }
  for (let part = 1; part <= session.totalChunks; part += 1) {
    if (receivedParts.has(part)) {
      onProgress(Math.round((receivedParts.size / session.totalChunks) * 90))
      continue
    }
    const chunk = file.slice((part - 1) * chunkSize, Math.min(part * chunkSize, file.size))
    await request.put({
      url: `/ai-platform/projects/${projectId}/uploads/${session.id}/chunks/${part}`,
      data: chunk,
      headersType: 'application/octet-stream',
      headers: { 'X-Chunk-SHA256': await sha256(chunk) }
    })
    receivedParts.add(part)
    onProgress(Math.round((receivedParts.size / session.totalChunks) * 90))
  }
  const result = await request.post<{ asset: Asset }>({
    url: `/ai-platform/projects/${projectId}/uploads/${session.id}/complete`
  })
  localStorage.removeItem(resumeKey)
  onProgress(100)
  return result.asset
}

export const analyzeAssetSimilarity = (projectId: number, distanceThreshold = 6) =>
  request.post<{
    name: string
    analyzedCount: number
    groups: Array<{ canonicalAssetId: number; assetIds: number[]; hashes: Record<string, string> }>
  }>({
    url: `/ai-platform/projects/${projectId}/assets/similarity`,
    params: { distanceThreshold }
  })
