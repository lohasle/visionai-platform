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
  templateId: number
  templateSnapshot: string
  templateSha256: string
  inputFingerprint: string
  currentStep: number
  totalSteps: number
  invalidatedAt?: string
  invalidationReason?: string
  submittedBy: number
  decidedBy: number
  submittedAt: string
}

export interface ModelLicenseDeclaration {
  id: number
  componentType: 'DATA' | 'PRETRAINED_WEIGHT' | 'FRAMEWORK' | 'MODEL_ARTIFACT'
  componentName: string
  licenseId: string
  sourceUri: string
  useDeclaration: string
  decision: 'ALLOWED' | 'REVIEW_REQUIRED' | 'NOT_ALLOWED'
  reviewedBy: number
}

export interface ApprovalTemplate {
  id: number
  name: string
  approvalType: string
  targetEnvironment: string
  steps: string
  allowSelfApproval: boolean
  enabled: boolean
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
    licenses: ModelLicenseDeclaration[]
    approvals: ApprovalRequest[]
    decisions: Array<{ id: number; decision: string; comment: string; decidedBy: number }>
    stepDecisions: Array<{
      id: number
      approvalRequestId: number
      stepNo: number
      stepName: string
      requiredRole: string
      decision: string
      comment: string
      decidedBy: number
    }>
  }>({ url: `/ai-platform/projects/${projectId}/model-versions/${versionId}` })

export const compareModelVersions = (projectId: number, ids: number[]) =>
  request.get<
    Array<{
      version: ModelVersion
      evaluationSummary: Record<string, unknown>
      trainingParameters: Record<string, unknown>
      runtimeSpec: Record<string, unknown>
      datasetVersion: { id: number; semanticVersion: string; checksum: string }
      artifactCount: number
      artifactBytes: number
      deploymentPerformance: {
        requests: number
        qps: number
        errorRate: number
        p50: number
        p95: number
        p99: number
      }
    }>
  >({
    url: `/ai-platform/projects/${projectId}/model-versions/compare`,
    params: { ids: ids.join(',') }
  })

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

export const getApprovalTemplates = () =>
  request.get<ApprovalTemplate[]>({ url: '/ai-platform/approval-templates?enabled=true' })

export const createApprovalTemplate = (data: Record<string, unknown>) =>
  request.post<ApprovalTemplate>({ url: '/ai-platform/approval-templates', data })

export const decideApproval = (
  projectId: number,
  approvalId: number,
  data: { decision: 'APPROVE' | 'REJECT'; comment: string }
) =>
  request.post({
    url: `/ai-platform/projects/${projectId}/approvals/${approvalId}/decision`,
    data
  })

export const cancelApproval = (projectId: number, approvalId: number, comment: string) =>
  request.post({
    url: `/ai-platform/projects/${projectId}/approvals/${approvalId}/cancel`,
    data: { comment }
  })

export const exportModel = (
  projectId: number,
  versionId: number,
  data: { purpose: string; includeArtifacts: boolean }
) =>
  request.post({
    url: `/ai-platform/projects/${projectId}/model-versions/${versionId}/export`,
    data
  })

export const replaceModelLicenses = (
  projectId: number,
  versionId: number,
  licenses: Array<Record<string, unknown>>
) =>
  request.put<ModelVersion>({
    url: `/ai-platform/projects/${projectId}/model-versions/${versionId}/licenses`,
    data: { licenses }
  })
