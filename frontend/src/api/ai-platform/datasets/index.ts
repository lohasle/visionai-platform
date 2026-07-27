import request from '@/config/axios'

export type DatasetVersionStatus = 'DRAFT' | 'VALIDATING' | 'READY' | 'FROZEN' | 'DEPRECATED'

export interface Dataset {
  id: number
  projectId: number
  name: string
  taskType: string
  description: string
  ownerUserId: number
  versionCount: number
  frozenCount: number
}

export interface DatasetVersion {
  id: number
  datasetId: number
  versionNo: number
  semanticVersion: string
  parentId: number
  sourceType: string
  sourceId: number
  annotationRevisionId: number
  ontologyVersionId: number
  ontologyVersion: string
  ontologyChecksum: string
  splitSeed: number
  status: DatasetVersionStatus
  itemCount: number
  trainCount: number
  validationCount: number
  testCount: number
  manifestUri: string
  datasetCardUri: string
  checksum: string
  validationSummary: string
  frozenAt: string
}

export interface DatasetValidationIssue {
  id: number
  assetId: number
  severity: 'ERROR' | 'WARNING'
  code: string
  message: string
  remediation: string
}

export const getDatasets = (projectId: number) =>
  request.get<Dataset[]>({ url: `/ai-platform/projects/${projectId}/datasets` })

export const createDataset = (
  projectId: number,
  data: { name: string; taskType: string; description: string; ownerUserId: number }
) => request.post<Dataset>({ url: `/ai-platform/projects/${projectId}/datasets`, data })

export const getDatasetVersions = (projectId: number, datasetId: number) =>
  request.get<DatasetVersion[]>({
    url: `/ai-platform/projects/${projectId}/datasets/${datasetId}/versions`
  })

export const createDatasetVersion = (
  projectId: number,
  datasetId: number,
  data: {
    sourceType: string
    sourceId: number
    annotationRevisionId: number
    ontologyVersionId: number
    splitSeed: number
    split: Record<string, number>
  }
) =>
  request.post<DatasetVersion>({
    url: `/ai-platform/projects/${projectId}/datasets/${datasetId}/versions`,
    data
  })

export const getDatasetVersion = (projectId: number, versionId: number) =>
  request.get<{
    version: DatasetVersion
    issues: DatasetValidationIssue[]
    usage: Array<{ resourceType: string; resourceId: number }>
  }>({ url: `/ai-platform/projects/${projectId}/dataset-versions/${versionId}` })

export const validateDatasetVersion = (projectId: number, versionId: number) =>
  request.post({
    url: `/ai-platform/projects/${projectId}/dataset-versions/${versionId}/validate`
  })

export const freezeDatasetVersion = (projectId: number, versionId: number) =>
  request.post({
    url: `/ai-platform/projects/${projectId}/dataset-versions/${versionId}/freeze`
  })

export const deprecateDatasetVersion = (projectId: number, versionId: number, reason: string) =>
  request.post({
    url: `/ai-platform/projects/${projectId}/dataset-versions/${versionId}/deprecate`,
    data: { reason }
  })

export const compareDatasetVersions = (projectId: number, left: number, right: number) =>
  request.get<{ added: number; removed: number; unchanged: number }>({
    url: `/ai-platform/projects/${projectId}/dataset-versions/compare`,
    params: { left, right }
  })
