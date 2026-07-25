import request from '@/config/axios'

export interface EvaluationSuite {
  id: number
  name: string
  datasetVersionId: number
  slices: string
  thresholds: string
  gatePolicy: string
  evaluatorVersion: string
}

export interface EvaluationRun {
  id: number
  suiteId: number
  trainingRunId: number
  baselineRunId: number
  status: string
  gateDecision: string
  summary: string
  fiftyOneDataset: string
  errorCode: string
  errorMessage: string
  createTime: string
}

export interface EvaluationMetric {
  id: number
  slice: string
  name: string
  value: number
}

export interface EvaluationSample {
  id: number
  assetId: number
  split: string
  slice: string
  errorType: string
  confidence: number
  iou: number
  gtCount: number
  predictionCount: number
  latencyMs: number
}

export const getEvaluationSuites = (projectId: number) =>
  request.get<EvaluationSuite[]>({ url: `/ai-platform/projects/${projectId}/evaluation-suites` })

export const createEvaluationSuite = (projectId: number, data: Record<string, unknown>) =>
  request.post<EvaluationSuite>({
    url: `/ai-platform/projects/${projectId}/evaluation-suites`,
    data
  })

export const getEvaluationRuns = (projectId: number) =>
  request.get<EvaluationRun[]>({ url: `/ai-platform/projects/${projectId}/evaluation-runs` })

export const createEvaluationRun = (
  projectId: number,
  suiteId: number,
  data: Record<string, unknown>
) =>
  request.post<{ run: EvaluationRun }>({
    url: `/ai-platform/projects/${projectId}/evaluation-suites/${suiteId}/runs`,
    data
  })

export const getEvaluationRun = (projectId: number, runId: number, errorType?: string) =>
  request.get<{
    run: EvaluationRun
    metrics: EvaluationMetric[]
    samples: EvaluationSample[]
    savedSlices: Array<{ id: number; name: string; sampleCount: number; filter: string }>
  }>({
    url: `/ai-platform/projects/${projectId}/evaluation-runs/${runId}`,
    params: { errorType }
  })

export const openEvaluationWorkbench = (projectId: number, runId: number) =>
  request.post<{ url: string; dataset: string }>({
    url: `/ai-platform/projects/${projectId}/evaluation-runs/${runId}/workbench`
  })
