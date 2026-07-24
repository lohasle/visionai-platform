import request from '@/config/axios'

export interface FeedbackPolicy {
  id: number
  enabled: boolean
  randomRate: number
  confidenceBelow: number
  captureEmpty: boolean
  captureErrors: boolean
  dailyLimit: number
  retentionDays: number
  redactionPolicy: string
  sensitiveReview: boolean
}

export interface FeedbackSample {
  id: number
  inferenceTraceId: number
  deploymentRevisionId: number
  modelVersionId: number
  assetId: number
  reason: string
  perceptualHash: string
  status: string
  batchId: number
  expiresAt: string
}

export interface FeedbackBatch {
  id: number
  name: string
  status: string
  sampleCount: number
  annotationTaskId: number
  annotationRevisionId: number
  datasetVersionId: number
  modelVersionId: number
  createTime: string
}

export const getFeedbackPolicy = (projectId: number) =>
  request.get<FeedbackPolicy | null>({ url: `/ai-platform/projects/${projectId}/feedback-policy` })
export const saveFeedbackPolicy = (projectId: number, data: Record<string, unknown>) =>
  request.put<FeedbackPolicy>({ url: `/ai-platform/projects/${projectId}/feedback-policy`, data })
export const getFeedbackSamples = (projectId: number, status?: string) =>
  request.get<FeedbackSample[]>({ url: `/ai-platform/projects/${projectId}/feedback-samples`, params: { status } })
export const getFeedbackBatches = (projectId: number) =>
  request.get<FeedbackBatch[]>({ url: `/ai-platform/projects/${projectId}/feedback-batches` })
export const createFeedbackBatch = (projectId: number, data: { name: string; sampleIds: number[] }) =>
  request.post<FeedbackBatch>({ url: `/ai-platform/projects/${projectId}/feedback-batches`, data })
export const reviewFeedbackBatch = (projectId: number, batchId: number, data: { decision: string; comment: string }) =>
  request.post({
    url: `/ai-platform/projects/${projectId}/feedback-batches/${batchId}/review`,
    data
  })
export const cleanupFeedback = (projectId: number) =>
  request.post<{ expired: number }>({ url: `/ai-platform/projects/${projectId}/feedback-cleanup` })
