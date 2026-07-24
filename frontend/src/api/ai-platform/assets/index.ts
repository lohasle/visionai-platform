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
  status: 'READY' | 'INVALID' | 'MISSING' | 'DELETED' | 'PURGED'
  duplicateOfId: number
  errorCode: string
  errorMessage: string
  createTime: string
}

export interface AssetPage {
  list: Asset[]
  total: number
}

export interface AssetQuality {
  byStatus: Array<{ status: string; count: number }>
  duplicateGroups: number
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

const sha256 = async (blob: Blob) => {
  const digest = await crypto.subtle.digest('SHA-256', await blob.arrayBuffer())
  return Array.from(new Uint8Array(digest))
    .map((value) => value.toString(16).padStart(2, '0'))
    .join('')
}

export const uploadAsset = async (
  projectId: number,
  file: File,
  onProgress: (progress: number) => void
) => {
  const chunkSize = 5 * 1024 * 1024
  const session = await request.post<{ id: string; totalChunks: number }>({
    url: `/ai-platform/projects/${projectId}/uploads`,
    data: {
      filename: file.name,
      contentType: file.type,
      totalSize: file.size,
      chunkSize,
      duplicatePolicy: 'REFERENCE'
    }
  })
  for (let part = 1; part <= session.totalChunks; part += 1) {
    const chunk = file.slice((part - 1) * chunkSize, Math.min(part * chunkSize, file.size))
    await request.put({
      url: `/ai-platform/projects/${projectId}/uploads/${session.id}/chunks/${part}`,
      data: chunk,
      headersType: 'application/octet-stream',
      headers: { 'X-Chunk-SHA256': await sha256(chunk) }
    })
    onProgress(Math.round((part / session.totalChunks) * 90))
  }
  const result = await request.post<{ asset: Asset }>({
    url: `/ai-platform/projects/${projectId}/uploads/${session.id}/complete`
  })
  onProgress(100)
  return result.asset
}
