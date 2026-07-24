import request from '@/config/axios'

export const getResourceOverview = () =>
  request.get<{
    nodes: Array<Record<string, any>>
    queues: Array<Record<string, any>>
    activeJobs: number
    queuedJobs: number
    storageBytes: number
  }>({ url: '/ai-platform/resources/overview' })
export const heartbeatNode = (data: Record<string, unknown>) =>
  request.post({ url: '/ai-platform/resources/nodes/heartbeat', data })
export const saveQueue = (data: Record<string, unknown>) =>
  request.put({ url: '/ai-platform/resources/queues', data })
export const saveQuota = (projectId: number, data: Record<string, unknown>) =>
  request.put({ url: `/ai-platform/projects/${projectId}/quota`, data })
export const getIntegrations = () =>
  request.get<{
    instances: Array<Record<string, any>>
    compatibilityRules: Array<Record<string, any>>
    incidents: Array<Record<string, any>>
  }>({ url: '/ai-platform/integrations' })
export const createIntegration = (data: Record<string, unknown>) =>
  request.post({ url: '/ai-platform/integrations', data })
export const testIntegration = (instanceId: number) =>
  request.post({ url: `/ai-platform/integrations/${instanceId}/test` })
export const createCompatibilityRule = (data: Record<string, unknown>) =>
  request.post({ url: '/ai-platform/compatibility-rules', data })
export const replayIncident = (incidentId: number) =>
  request.post({ url: `/ai-platform/sync-incidents/${incidentId}/replay` })
export const getAuditEvents = (params?: Record<string, unknown>) =>
  request.get<Array<Record<string, any>>>({ url: '/ai-platform/audit-events', params })
