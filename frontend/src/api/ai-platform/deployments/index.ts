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
    traces: Array<{
      id: number
      traceId: string
      assetId: number
      status: string
      latencyMs: number
      detectionCount: number
      meanConfidence: number
      deploymentRevisionId: number
      modelVersionId: number
    }>
    metrics: Record<string, number>
    alertRules: Array<{ id: number; name: string; metric: string; operator: string; threshold: number }>
    alerts: Array<{ id: number; status: string; message: string; metricValue: number; createTime: string }>
  }>({ url: `/ai-platform/projects/${projectId}/deployments/${deploymentId}` })

export const predict = (projectId: number, deploymentId: number, assetId: number) =>
  request.post<{
    traceId: string
    deploymentRevisionId: number
    modelVersionId: number
    result: { detections: Array<{ label: string; confidence: number; bbox: number[] }>; latencyMs: number }
  }>({ url: `/ai-platform/projects/${projectId}/deployments/${deploymentId}/predict`, data: { assetId } })

export const rollbackDeployment = (projectId: number, deploymentId: number, targetRevisionId: number) =>
  request.post({
    url: `/ai-platform/projects/${projectId}/deployments/${deploymentId}/rollback`,
    data: { targetRevisionId }
  })

export const stopDeployment = (projectId: number, deploymentId: number) =>
  request.post({ url: `/ai-platform/projects/${projectId}/deployments/${deploymentId}/stop` })

export const createAlertRule = (projectId: number, deploymentId: number, data: Record<string, unknown>) =>
  request.post({
    url: `/ai-platform/projects/${projectId}/deployments/${deploymentId}/alert-rules`,
    data
  })

export const acknowledgeAlert = (projectId: number, alertId: number) =>
  request.post({ url: `/ai-platform/projects/${projectId}/alerts/${alertId}/acknowledge` })
