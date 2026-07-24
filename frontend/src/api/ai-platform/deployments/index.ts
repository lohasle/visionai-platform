import request from '@/config/axios'

export interface Deployment {
  id: number
  name: string
  environment: string
  status: string
  currentRevisionId: number
  endpointUrl: string
  createTime: string
}

export interface DeploymentRevision {
  id: number
  deploymentId: number
  revisionNo: number
  modelVersionId: number
  sourceRevisionId: number
  status: string
  config: string
  artifactSha256: string
  activatedAt?: string
}

export interface InferenceStatus {
  ready: boolean
  deploymentStatus: string
  revisionStatus?: string
  currentRevisionId: number
  revisionNo?: number
  modelVersionId?: number
  endpointUrl: string
  checkedAt: string
  lastTraceId?: string
  lastRequestAt?: string
  lastRequestStatus?: string
  lastLatencyMs?: number
  lastErrorAt?: string
  lastErrorMessage?: string
}

export interface InferenceTrace {
  id: number
  traceId: string
  assetId: number
  sourceType: 'ASSET' | 'UPLOAD' | 'VIDEO_UPLOAD'
  sourceName: string
  sourceSha256: string
  testMode: 'ONLINE' | 'REGRESSION'
  expectedLabel: string
  minimumConfidence: number
  regressionStatus: 'NOT_ASSERTED' | 'PASSED' | 'FAILED'
  matchedDetectionCount: number
  status: string
  latencyMs: number
  detectionCount: number
  meanConfidence: number
  deploymentRevisionId: number
  modelVersionId: number
  errorMessage: string
  createTime: string
}

export interface PredictionResponse {
  traceId: string
  deploymentRevisionId: number
  modelVersionId: number
  platformLatencyMs: number
  result: {
    detections: Array<{ label: string; confidence: number; bbox: number[] }>
    latencyMs: number
  }
}

export interface ImageRegressionResponse extends PredictionResponse {
  input: { filename: string; format: string; width: number; height: number; sha256: string }
  regression: {
    status: 'NOT_ASSERTED' | 'PASSED' | 'FAILED'
    expectedLabel: string
    minimumConfidence: number
    matchedDetectionCount: number
  }
}

export const getDeployments = (projectId: number) =>
  request.get<{ deployments: Deployment[]; revisions: DeploymentRevision[] }>({
    url: `/ai-platform/projects/${projectId}/deployments`
  })

export const createDeployment = (projectId: number, data: Record<string, unknown>) =>
  request.post({ url: `/ai-platform/projects/${projectId}/deployments`, data })

export const getDeployment = (projectId: number, deploymentId: number) =>
  request.get<{
    deployment: Deployment
    revisions: DeploymentRevision[]
    traces: InferenceTrace[]
    metrics: Record<string, number>
    inferenceStatus: InferenceStatus
    alertRules: Array<{
      id: number
      name: string
      metric: string
      operator: string
      threshold: number
    }>
    alerts: Array<{
      id: number
      status: string
      message: string
      metricValue: number
      createTime: string
    }>
  }>({ url: `/ai-platform/projects/${projectId}/deployments/${deploymentId}` })

export const predict = (projectId: number, deploymentId: number, assetId: number) =>
  request.post<PredictionResponse>({
    url: `/ai-platform/projects/${projectId}/deployments/${deploymentId}/predict`,
    data: { assetId }
  })

export const predictImage = (
  projectId: number,
  deploymentId: number,
  file: File,
  expectedLabel?: string,
  minimumConfidence?: number
) => {
  const data = new FormData()
  data.append('file', file)
  if (expectedLabel?.trim()) data.append('expectedLabel', expectedLabel.trim())
  if (minimumConfidence !== undefined) data.append('minimumConfidence', String(minimumConfidence))
  return request.postMultipart<ImageRegressionResponse>({
    url: `/ai-platform/projects/${projectId}/deployments/${deploymentId}/predict-image`,
    data
  })
}

export const rollbackDeployment = (
  projectId: number,
  deploymentId: number,
  targetRevisionId: number
) =>
  request.post({
    url: `/ai-platform/projects/${projectId}/deployments/${deploymentId}/rollback`,
    data: { targetRevisionId }
  })

export const stopDeployment = (projectId: number, deploymentId: number) =>
  request.post({ url: `/ai-platform/projects/${projectId}/deployments/${deploymentId}/stop` })

export const restartDeployment = (projectId: number, deploymentId: number) =>
  request.post({ url: `/ai-platform/projects/${projectId}/deployments/${deploymentId}/restart` })

export const createAlertRule = (
  projectId: number,
  deploymentId: number,
  data: Record<string, unknown>
) =>
  request.post({
    url: `/ai-platform/projects/${projectId}/deployments/${deploymentId}/alert-rules`,
    data
  })

export const acknowledgeAlert = (projectId: number, alertId: number) =>
  request.post({ url: `/ai-platform/projects/${projectId}/alerts/${alertId}/acknowledge` })
