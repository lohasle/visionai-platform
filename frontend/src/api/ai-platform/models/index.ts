import request from '@/config/axios'

export interface ModelVersion {
  id: number
  modelId: number
  versionNo: number
  semanticVersion: string
  sourceType: string
  trainingRunId: number
  evaluationRunId: number
  datasetVersionId: number
  status: string
  modelCardUri: string
  modelCardSha256: string
  supplyChainUri: string
  supplyChainSha256: string
  licenseDecision: string
  preannotationApproved: boolean
  createdBy: number
  approvedBy: number
  approvedAt?: string
  createTime: string
}

export interface ModelRecord {
  id: number
  name: string
  aiType: string
  description: string
}

export interface ApprovalRequest {
  id: number
  modelVersionId: number
  approvalType: string
  targetEnvironment: string
  status: string
  evidenceSha256: string
  submittedBy: number
  decidedBy: number
  submittedAt: string
}

export const getModels = (projectId: number) =>
  request.get<{ models: ModelRecord[]; versions: ModelVersion[] }>({
    url: `/ai-platform/projects/${projectId}/models`
  })

export const registerModel = (projectId: number, data: Record<string, unknown>) =>
  request.post<ModelVersion>({ url: `/ai-platform/projects/${projectId}/models/register`, data })

export const getModelVersion = (projectId: number, versionId: number) =>
  request.get<{
    version: ModelVersion
    artifacts: Array<{ id: number; format: string; uri: string; sha256: string; size: number }>
    approvals: ApprovalRequest[]
    decisions: Array<{ id: number; decision: string; comment: string; decidedBy: number }>
  }>({ url: `/ai-platform/projects/${projectId}/model-versions/${versionId}` })

export const submitApproval = (
  projectId: number,
  versionId: number,
  data: Record<string, unknown>
) =>
  request.post<ApprovalRequest>({
    url: `/ai-platform/projects/${projectId}/model-versions/${versionId}/approvals`,
    data
  })

export const getApprovals = (projectId: number) =>
  request.get<ApprovalRequest[]>({ url: `/ai-platform/projects/${projectId}/approvals` })

export const decideApproval = (
  projectId: number,
  approvalId: number,
  data: { decision: 'APPROVE' | 'REJECT'; comment: string }
) =>
  request.post({
    url: `/ai-platform/projects/${projectId}/approvals/${approvalId}/decision`,
    data
  })

export const exportModelManifest = (projectId: number, versionId: number) =>
  request.get({
    url: `/ai-platform/projects/${projectId}/model-versions/${versionId}/export-manifest`
  })
