import request from '@/config/axios'

export const getResourceOverview = () =>
  request.get<{
    nodes: Array<Record<string, any>>
    queues: Array<
      Record<string, any> & {
        queuedJobs: number
        runningJobs: number
        averageWaitSeconds: number
        completedJobs: number
      }
    >
    activeJobs: number
    queuedJobs: number
    storageBytes: number
    storageObjects: number
    storageFailures: number
    storageCapacityBytes: number
    storageUtilization: number
    lifecyclePolicies: number
    lifecyclePurged: number
    gpuHours: number
  }>({ url: '/ai-platform/resources/overview' })
export const getResourceHistory = (hours = 24, nodeKey?: string) =>
  request.get<Array<Record<string, any>>>({
    url: '/ai-platform/resources/history',
    params: { hours, nodeKey }
  })
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
  request.post<{
    success: boolean
    message: string
    latencyMs: number
    checks: Array<{ name: string; success: boolean; latencyMs: number; detail: string }>
  }>({ url: `/ai-platform/integrations/${instanceId}/test` })
export const getIntegrationRevisions = (instanceId: number) =>
  request.get<Array<Record<string, any>>>({
    url: `/ai-platform/integrations/${instanceId}/revisions`
  })
export const upgradeIntegration = (instanceId: number, data: Record<string, unknown>) =>
  request.post<{
    instance: Record<string, any>
    revision: Record<string, any>
    rolledBack: boolean
  }>({ url: `/ai-platform/integrations/${instanceId}/upgrade`, data })
export const createCompatibilityRule = (data: Record<string, unknown>) =>
  request.post({ url: '/ai-platform/compatibility-rules', data })
export const replayIncident = (incidentId: number) =>
  request.post({ url: `/ai-platform/sync-incidents/${incidentId}/replay` })
export const actOnIncident = (
  incidentId: number,
  data: { action: 'REPLAY' | 'REBIND' | 'IGNORE' | 'CLOSE'; newExternalId?: string; reason: string }
) => request.post({ url: `/ai-platform/sync-incidents/${incidentId}/action`, data })
export const getAuditEvents = (params?: Record<string, unknown>) =>
  request.get<Array<Record<string, any>>>({ url: '/ai-platform/audit-events', params })
export const exportAuditEvents = (params?: Record<string, unknown>) =>
  request.download({ url: '/ai-platform/audit-events/export', params })
