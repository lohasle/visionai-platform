import request from '@/config/axios'

export type OntologyVersionStatus = 'DRAFT' | 'PUBLISHED' | 'DEPRECATED'
export type TagCategory = 'BUSINESS' | 'SCENE' | 'SOURCE'

export interface Ontology {
  id: number
  projectId: number
  code: string
  name: string
  taskType: string
  description: string
  status: 'ACTIVE' | 'ARCHIVED'
  versionCount: number
  publishedCount: number
  latestVersionId: number
  latestVersion: string
  latestVersionStatus: OntologyVersionStatus
}

export interface OntologyVersion {
  id: number
  ontologyId: number
  versionNo: number
  semanticVersion: string
  status: OntologyVersionStatus
  checksum: string
  publishedAt?: string
}

export interface SelectableOntologyVersion extends OntologyVersion {
  ontologyCode: string
  ontologyName: string
  taskType: string
  labelCount: number
}

export interface OntologyAttribute {
  id?: number
  name: string
  inputType: string
  values: string[] | string
  defaultValue: string
  mutable: boolean
  sort: number
}

export interface OntologyLabel {
  id?: number
  code: string
  name: string
  color: string
  shapeType: string
  sort: number
  attributes: OntologyAttribute[]
}

export interface OntologyDetail {
  ontology: Ontology
  versions: OntologyVersion[]
}

export interface OntologyVersionDetail {
  ontology: Ontology
  version: OntologyVersion
  labels: OntologyLabel[]
}

export interface AssetTagDefinition {
  id: number
  projectId: number
  code: string
  name: string
  category: TagCategory
  color: string
  description: string
  enabled: boolean
  usageCount: number
}

export const getOntologies = (projectId: number) =>
  request.get<Ontology[]>({ url: `/ai-platform/projects/${projectId}/ontologies` })

export const createOntology = (
  projectId: number,
  data: { code: string; name: string; taskType: string; description: string }
) =>
  request.post<{ ontology: Ontology; version: OntologyVersion }>({
    url: `/ai-platform/projects/${projectId}/ontologies`,
    data
  })

export const getOntology = (projectId: number, ontologyId: number) =>
  request.get<OntologyDetail>({
    url: `/ai-platform/projects/${projectId}/ontologies/${ontologyId}`
  })

export const createOntologyVersion = (
  projectId: number,
  ontologyId: number,
  sourceVersionId?: number
) =>
  request.post<OntologyVersion>({
    url: `/ai-platform/projects/${projectId}/ontologies/${ontologyId}/versions`,
    data: { sourceVersionId: sourceVersionId || 0 }
  })

export const getOntologyVersion = (projectId: number, versionId: number) =>
  request.get<OntologyVersionDetail>({
    url: `/ai-platform/projects/${projectId}/ontology-versions/${versionId}`
  })

export const getPublishedOntologyVersions = (projectId: number, taskType = 'CV_DETECTION') =>
  request.get<SelectableOntologyVersion[]>({
    url: `/ai-platform/projects/${projectId}/ontology-versions`,
    params: { status: 'PUBLISHED', taskType }
  })

export const replaceOntologyLabels = (
  projectId: number,
  versionId: number,
  labels: OntologyLabel[]
) =>
  request.put<OntologyLabel[]>({
    url: `/ai-platform/projects/${projectId}/ontology-versions/${versionId}/labels`,
    data: { labels }
  })

export const publishOntologyVersion = (projectId: number, versionId: number) =>
  request.post<OntologyVersion>({
    url: `/ai-platform/projects/${projectId}/ontology-versions/${versionId}/publish`
  })

export const deprecateOntologyVersion = (projectId: number, versionId: number) =>
  request.post<OntologyVersion>({
    url: `/ai-platform/projects/${projectId}/ontology-versions/${versionId}/deprecate`
  })

export const getTagDefinitions = (projectId: number) =>
  request.get<AssetTagDefinition[]>({
    url: `/ai-platform/projects/${projectId}/tag-definitions`
  })

export const createTagDefinition = (
  projectId: number,
  data: Omit<AssetTagDefinition, 'id' | 'projectId' | 'usageCount'>
) =>
  request.post<AssetTagDefinition>({
    url: `/ai-platform/projects/${projectId}/tag-definitions`,
    data
  })

export const updateTagDefinition = (
  projectId: number,
  definitionId: number,
  data: Omit<AssetTagDefinition, 'id' | 'projectId' | 'usageCount'>
) =>
  request.put<AssetTagDefinition>({
    url: `/ai-platform/projects/${projectId}/tag-definitions/${definitionId}`,
    data
  })

export const updateAssetTags = (
  projectId: number,
  data: { assetIds: number[]; definitionIds: number[]; mode: 'ADD' | 'REPLACE' | 'REMOVE' }
) =>
  request.put({
    url: `/ai-platform/projects/${projectId}/assets/tags`,
    data
  })
