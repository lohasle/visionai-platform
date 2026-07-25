import request from '@/config/axios'

export type AnnotationStatus =
  | 'DRAFT'
  | 'PREPARING'
  | 'PREANNOTATING'
  | 'READY'
  | 'ANNOTATING'
  | 'REVIEWING'
  | 'APPROVED'
  | 'REJECTED'
  | 'EXPORTING'
  | 'CLOSED'
  | 'FAILED'
  | 'CANCELLED'

export interface AnnotationTask {
  id: number
  projectId: number
  name: string
  taskType: string
  collectionId: number
  ontologyVersion: string
  status: AnnotationStatus
  progress: number
  externalBindingId: number
  currentRevisionId: number
  rejectionCode: string
  rejectionReason: string
  errorCode: string
  errorMessage: string
  createTime: string
  updateTime: string
}

export interface AnnotationRevision {
  id: number
  revisionNo: number
  snapshotUri: string
  format: string
  checksum: string
  annotationCount: number
  createTime: string
}

export interface ExternalBinding {
  id: number
  externalId: string
  externalUrl: string
  syncCursor: string
  lastSyncAt: string
}

export interface CVATUserMapping {
  id: number
  platformUserId: number
  cvatUserId: number
  cvatUsername: string
  active: boolean
  verifiedAt: string
}

export interface AssetCollection {
  id: number
  name: string
  description: string
  frozen: boolean
  version: number
  assetCount: number
}

export const getAnnotationTasks = (projectId: number, params: Record<string, unknown>) =>
  request.get<{ list: AnnotationTask[]; total: number }>({
    url: `/ai-platform/projects/${projectId}/annotation-tasks`,
    params
  })

export const getAnnotationTask = (projectId: number, taskId: number) =>
  request.get<{
    task: AnnotationTask
    binding: ExternalBinding
    revisions: AnnotationRevision[]
  }>({ url: `/ai-platform/projects/${projectId}/annotation-tasks/${taskId}` })

export const createAnnotationTask = (
  projectId: number,
  data: {
    name: string
    taskType: string
    collectionId: number
    ontologyVersion: string
    labels: Array<{ name: string; color: string; type: string }>
    annotatorIds: number[]
    reviewerIds: number[]
  }
) =>
  request.post<AnnotationTask>({ url: `/ai-platform/projects/${projectId}/annotation-tasks`, data })

export const prepareAnnotationTask = (projectId: number, taskId: number) =>
  request.post({ url: `/ai-platform/projects/${projectId}/annotation-tasks/${taskId}/prepare` })

export const syncAnnotationTask = (projectId: number, taskId: number) =>
  request.post({ url: `/ai-platform/projects/${projectId}/annotation-tasks/${taskId}/sync` })

export const updateAnnotationStatus = (
  projectId: number,
  taskId: number,
  status: AnnotationStatus
) =>
  request.put<AnnotationTask>({
    url: `/ai-platform/projects/${projectId}/annotation-tasks/${taskId}/status`,
    data: { status }
  })

export const reviewAnnotationTask = (
  projectId: number,
  taskId: number,
  data: { decision: 'APPROVE' | 'REJECT'; code?: string; reason?: string }
) =>
  request.post<AnnotationTask>({
    url: `/ai-platform/projects/${projectId}/annotation-tasks/${taskId}/review`,
    data
  })

export const exportAnnotationTask = (projectId: number, taskId: number) =>
  request.post({ url: `/ai-platform/projects/${projectId}/annotation-tasks/${taskId}/export` })

export const openAnnotationWorkbench = (projectId: number, taskId: number) =>
  request.post<{ url: string }>({
    url: `/ai-platform/projects/${projectId}/annotation-tasks/${taskId}/workbench`
  })

export const getCVATUserMappings = () =>
  request.get<CVATUserMapping[]>({ url: '/ai-platform/cvat-user-mappings' })

export const saveCVATUserMapping = (data: { platformUserId: number }) =>
  request.put<CVATUserMapping>({ url: '/ai-platform/cvat-user-mappings', data })

export const getAssetCollections = (projectId: number) =>
  request.get<AssetCollection[]>({ url: `/ai-platform/projects/${projectId}/collections` })
