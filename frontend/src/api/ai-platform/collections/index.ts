import request from '@/config/axios'
import type { Asset } from '@/api/ai-platform/assets'

export interface AssetCollection {
  id: number
  projectId: number
  name: string
  description: string
  frozen: boolean
  version: number
  assetCount: number
  createTime: string
  updateTime: string
}

export interface CollectionAssetPage {
  collection: AssetCollection
  list: Array<Asset & { thumbnailUrl: string }>
  total: number
}

export const getCollections = (projectId: number) =>
  request.get<AssetCollection[]>({ url: `/ai-platform/projects/${projectId}/collections` })

export const createCollection = (
  projectId: number,
  data: { name: string; description: string; filter: Record<string, unknown> }
) => request.post<AssetCollection>({ url: `/ai-platform/projects/${projectId}/collections`, data })

export const getCollectionAssets = (
  projectId: number,
  collectionId: number,
  params: Record<string, unknown>
) =>
  request.get<CollectionAssetPage>({
    url: `/ai-platform/projects/${projectId}/collections/${collectionId}/assets`,
    params
  })

export const addCollectionAssets = (projectId: number, collectionId: number, assetIds: number[]) =>
  request.put({
    url: `/ai-platform/projects/${projectId}/collections/${collectionId}/assets`,
    data: { assetIds }
  })

export const freezeCollection = (projectId: number, collectionId: number) =>
  request.post<AssetCollection>({
    url: `/ai-platform/projects/${projectId}/collections/${collectionId}/freeze`
  })
