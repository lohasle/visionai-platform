import request from '@/config/axios'

export interface TrainingTemplate {
  id: number
  projectId: number
  name: string
  aiType: string
  description: string
  versionCount: number
  publishedCount: number
}

export interface TrainingTemplateVersion {
  id: number
  templateId: number
  versionNo: number
  semanticVersion: string
  trainer: string
  imageRef: string
  entrypoint: string
  parameterSchema: string
  resourceRequirements: string
  compatibility: string
  licensePolicy: string
  sourceYaml: string
  sourceYamlSha256: string
  outputProtocol: string
  published: boolean
  smokeStatus: 'PENDING' | 'PASSED' | 'FAILED'
  smokeReport: string
  createTime: string
}

export type TrainingStatus =
  | 'DRAFT'
  | 'QUEUED'
  | 'ALLOCATING'
  | 'RUNNING'
  | 'EXPORTING'
  | 'SUCCEEDED'
  | 'FAILED'
  | 'CANCELLED'
  | 'TIMEOUT'

export interface TrainingRun {
  id: number
  name: string
  datasetVersionId: number
  templateVersionId: number
  provider: string
  queue: string
  gpuCount: number
  priority: number
  status: TrainingStatus
  progress: number
  metricSummary: string
  errorCategory: string
  errorCode: string
  errorMessage: string
  resultManifestUri: string
  createTime: string
}

export interface TrainingArtifact {
  id: number
  kind: string
  name: string
  uri: string
  sha256: string
  size: number
  mediaType: string
}

export interface TrainingMetric {
  id: number
  name: string
  step: number
  value: number
}

export const getTrainingTemplates = (projectId: number) =>
  request.get<TrainingTemplate[]>({
    url: `/ai-platform/projects/${projectId}/training-templates`
  })

export const createTrainingTemplate = (
  projectId: number,
  data: { name: string; aiType: string; description: string }
) =>
  request.post<TrainingTemplate>({
    url: `/ai-platform/projects/${projectId}/training-templates`,
    data
  })

export const getTrainingTemplateVersions = (projectId: number, templateId: number) =>
  request.get<TrainingTemplateVersion[]>({
    url: `/ai-platform/projects/${projectId}/training-templates/${templateId}/versions`
  })

export const createTrainingTemplateVersion = (
  projectId: number,
  templateId: number,
  data: Record<string, unknown>
) =>
  request.post<TrainingTemplateVersion>({
    url: `/ai-platform/projects/${projectId}/training-templates/${templateId}/versions`,
    data
  })

export const smokeTrainingTemplateVersion = (projectId: number, versionId: number) =>
  request.post<TrainingTemplateVersion>({
    url: `/ai-platform/projects/${projectId}/training-template-versions/${versionId}/smoke`
  })

export const publishTrainingTemplateVersion = (projectId: number, versionId: number) =>
  request.post<TrainingTemplateVersion>({
    url: `/ai-platform/projects/${projectId}/training-template-versions/${versionId}/publish`
  })

export const getTrainingRuns = (projectId: number, filters: Record<string, unknown> = {}) =>
  request.get<{ list: TrainingRun[]; total: number }>({
    url: `/ai-platform/projects/${projectId}/training-runs`,
    params: { pageNo: 1, pageSize: 100, ...filters }
  })

export const createTrainingRun = (projectId: number, data: Record<string, unknown>) =>
  request.post<{ run: TrainingRun }>({
    url: `/ai-platform/projects/${projectId}/training-runs`,
    data
  })

export const getTrainingRun = (projectId: number, runId: number) =>
  request.get<{ run: TrainingRun; artifacts: TrainingArtifact[]; metrics: TrainingMetric[] }>({
    url: `/ai-platform/projects/${projectId}/training-runs/${runId}`
  })

export const compareTrainingRuns = (projectId: number, ids: number[]) =>
  request.get<Array<TrainingRun & { metrics: Record<string, number>; artifactCount: number }>>({
    url: `/ai-platform/projects/${projectId}/training-runs/compare`,
    params: { ids: ids.join(',') }
  })

export const cancelTrainingRun = (projectId: number, runId: number) =>
  request.post<TrainingRun>({
    url: `/ai-platform/projects/${projectId}/training-runs/${runId}/cancel`
  })

export const cloneTrainingRun = (projectId: number, runId: number) =>
  request.post<TrainingRun>({
    url: `/ai-platform/projects/${projectId}/training-runs/${runId}/clone`,
    data: {}
  })

export const downloadTrainingArtifact = (projectId: number, runId: number, artifactId: number) =>
  request.download({
    url: `/ai-platform/projects/${projectId}/training-runs/${runId}/artifacts/${artifactId}/download`
  })

export const exportTrainingRun = (projectId: number, runId: number) =>
  request.download({
    url: `/ai-platform/projects/${projectId}/training-runs/${runId}/export`
  })
