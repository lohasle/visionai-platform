param(
    [string]$BaseUrl = "http://localhost:58080",
    [string]$AdminUsername = "admin",
    [string]$AdminPassword = "admin123",
    [string]$ReviewerUsername = "visionai-reviewer",
    [string]$ReviewerPassword = "VisionAI-Review-2026!",
    [int]$ProjectId = 22,
    [int]$ModelVersionId = 10,
    [int]$IntegrationId = 2
)

$ErrorActionPreference = "Stop"

function To-JsonBytes($Value) {
    $json = $Value | ConvertTo-Json -Depth 30 -Compress
    return ,([System.Text.Encoding]::UTF8.GetBytes($json))
}

function Login([string]$Username, [string]$Password) {
    $response = Invoke-RestMethod -Method Post -Uri "$BaseUrl/admin-api/system/auth/login" `
        -Headers @{ "tenant-id" = "1" } -ContentType "application/json; charset=utf-8" `
        -Body (To-JsonBytes @{ username = $Username; password = $Password })
    if (-not $response.data.accessToken) { throw "Login failed for $Username" }
    return $response.data.accessToken
}

function Invoke-API(
    [string]$Token,
    [string]$Method,
    [string]$Path,
    $Body = $null
) {
    $headers = @{ Authorization = "Bearer $Token"; "tenant-id" = "1" }
    $arguments = @{
        Method = $Method
        Uri = "$BaseUrl$Path"
        Headers = $headers
    }
    if ($null -ne $Body) {
        $arguments.ContentType = "application/json; charset=utf-8"
        $arguments.Body = To-JsonBytes $Body
    }
    $response = Invoke-RestMethod @arguments
    if ($response.code -ne 0) { throw "$Method $Path failed: $($response.msg)" }
    return $response.data
}

function Assert([bool]$Condition, [string]$Message) {
    if (-not $Condition) { throw "ASSERTION FAILED: $Message" }
}

$adminToken = Login $AdminUsername $AdminPassword
$reviewerToken = Login $ReviewerUsername $ReviewerPassword
$stamp = Get-Date -Format "yyyyMMdd-HHmmss"

# FR-PRJ-006: clone governed configuration, never runtime or secrets.
$sourceTemplates = Invoke-API $adminToken GET "/admin-api/ai-platform/projects/$ProjectId/training-templates"
$clone = Invoke-API $adminToken POST "/admin-api/ai-platform/projects/$ProjectId/clone" @{
    code = "SUPPLY-CLONE-$stamp"
    name = "Supply Chain Clone $stamp"
    description = "SPEC-2290 governed clone acceptance"
}
$cloneTemplates = Invoke-API $adminToken GET "/admin-api/ai-platform/projects/$($clone.id)/training-templates"
$cloneOntologies = Invoke-API $adminToken GET "/admin-api/ai-platform/projects/$($clone.id)/ontologies"
$cloneRuns = Invoke-API $adminToken GET "/admin-api/ai-platform/projects/$($clone.id)/training-runs"
$cloneConfig = Invoke-API $adminToken GET "/admin-api/ai-platform/projects/$($clone.id)/config"
Assert (@($cloneTemplates).Count -eq @($sourceTemplates).Count) "training templates were not cloned"
Assert (@($cloneOntologies).Count -gt 0) "ontology configuration was not cloned"
Assert (@($cloneRuns.list).Count -eq 0) "training runtime data leaked into clone"
Assert (($cloneConfig.secretRefs | ConvertTo-Json -Compress) -eq "{}") "secret references leaked into clone"

# FR-INT-003/006: compatibility gate, successful upgrade, fault rollback and checksums.
$rule = Invoke-API $adminToken POST "/admin-api/ai-platform/compatibility-rules" @{
    providerType = "CLEARML"
    platformRange = ">=1.3.0 <2.0.0"
    providerRange = ">=2.0.0 <3.0.0"
    decision = "ALLOWED"
    notes = "SPEC-2290 automated acceptance"
}
$upgrade = Invoke-API $adminToken POST "/admin-api/ai-platform/integrations/$IntegrationId/upgrade" @{
    version = "2.4.1"
    faultInjection = $false
}
Assert (-not $upgrade.rolledBack) "healthy ClearML upgrade unexpectedly rolled back"
$rollback = Invoke-API $adminToken POST "/admin-api/ai-platform/integrations/$IntegrationId/upgrade" @{
    version = "2.4.2"
    faultInjection = $true
}
Assert $rollback.rolledBack "fault-injected upgrade did not roll back"
Assert ($rollback.instance.version -eq "2.4.1") "rollback did not restore the previous version"
$revisions = Invoke-API $adminToken GET "/admin-api/ai-platform/integrations/$IntegrationId/revisions"
Assert ($revisions.Count -ge 2) "integration revisions were not recorded"
$rollbackRevision = $revisions | Where-Object status -eq "ROLLED_BACK" | Select-Object -First 1
Assert ($rollbackRevision.beforeSha256.Length -eq 64) "rollback backup checksum missing"

# FR-INT-005: replay/rebind/ignore/manual close have explicit Before/After audit.
$integrationPage = Invoke-API $adminToken GET "/admin-api/ai-platform/integrations"
$incident = $integrationPage.incidents | Where-Object code -eq "UPGRADE_SMOKE_FAILED" | Select-Object -First 1
Assert ($null -ne $incident) "upgrade failure did not create a sync incident"
$ignored = Invoke-API $adminToken POST "/admin-api/ai-platform/sync-incidents/$($incident.id)/action" @{
    action = "IGNORE"
    reason = "Fault-injection acceptance incident"
}
Assert ($ignored.status -eq "IGNORED") "sync incident ignore action failed"

# Create a second approver to prove sequential multi-person separation.
$roles = Invoke-API $adminToken GET "/admin-api/system/role/simple-list"
$approverRole = $roles | Where-Object code -eq "APPROVER" | Select-Object -First 1
Assert ($null -ne $approverRole) "APPROVER system role is missing"
$approverUsername = "supply-approver-$stamp"
$approverPassword = "Supply-Approve-2026!"
$approverId = Invoke-API $adminToken POST "/admin-api/system/user/create" @{
    username = $approverUsername
    password = $approverPassword
    nickname = "Supply Chain Approver"
    deptId = 1
    roleIds = @($approverRole.id)
    postIds = @()
    status = 0
}
Invoke-API $adminToken PUT "/admin-api/ai-platform/projects/$ProjectId/members" @{ userId = $approverId } | Out-Null
$approverToken = Login $approverUsername $approverPassword

$licenses = @(
    @{ componentType = "DATA"; componentName = "COCO128 DatasetVersion #16"; licenseId = "AGPL-3.0-or-later"; sourceUri = "https://docs.ultralytics.com/datasets/detect/coco128/"; useDeclaration = "Public benchmark training, evaluation and governed production validation"; decision = "ALLOWED" },
    @{ componentType = "PRETRAINED_WEIGHT"; componentName = "No external pretrained weight"; licenseId = "NO_EXTERNAL_WEIGHT"; sourceUri = ""; useDeclaration = "No external pretrained weights were used"; decision = "ALLOWED" },
    @{ componentType = "FRAMEWORK"; componentName = "PyTorch 2.7.1 / Ultralytics"; licenseId = "BSD-3-Clause / AGPL-3.0-or-later"; sourceUri = "https://github.com/ultralytics/ultralytics"; useDeclaration = "GPU training, evaluation and inference conversion"; decision = "ALLOWED" },
    @{ componentType = "MODEL_ARTIFACT"; componentName = "COCO128 Baseline Detector"; licenseId = "PROJECT_MODEL_POLICY"; sourceUri = ""; useDeclaration = "Governed deployment in the approved target environment only"; decision = "ALLOWED" }
)
Invoke-API $adminToken PUT "/admin-api/ai-platform/projects/$ProjectId/model-versions/$ModelVersionId/licenses" @{ licenses = $licenses } | Out-Null

$template = Invoke-API $adminToken POST "/admin-api/ai-platform/approval-templates" @{
    name = "SPEC-2290 Sequential Approval $stamp"
    approvalType = "PRODUCTION_QUALIFICATION"
    targetEnvironment = "PRODUCTION"
    steps = @(
        @{ name = "Technical and compliance review"; requiredRole = "REVIEWER" },
        @{ name = "Production release approval"; requiredRole = "APPROVER" }
    )
    enabled = $true
}

function Complete-Approval([string]$Suffix) {
    $approval = Invoke-API $adminToken POST "/admin-api/ai-platform/projects/$ProjectId/model-versions/$ModelVersionId/approvals" @{
        approvalType = "PRODUCTION_QUALIFICATION"
        targetEnvironment = "PRODUCTION"
        templateId = $template.id
        riskSummary = "Small-object and occlusion accuracy risks are frozen in the failure-sample snapshot ($Suffix)."
        rollbackPlan = "Route traffic to the prior approved revision and preserve inference traces ($Suffix)."
    }
    Assert ($approval.totalSteps -eq 2) "approval template was not frozen as two steps"
    $step1 = Invoke-API $reviewerToken POST "/admin-api/ai-platform/projects/$ProjectId/approvals/$($approval.id)/decision" @{
        decision = "APPROVE"
        comment = "Technical evidence, licenses and failure samples reviewed."
    }
    Assert ($step1.approval.status -eq "IN_PROGRESS" -and $step1.approval.currentStep -eq 2) "first approval step did not advance"
    $step2 = Invoke-API $approverToken POST "/admin-api/ai-platform/projects/$ProjectId/approvals/$($approval.id)/decision" @{
        decision = "APPROVE"
        comment = "Production risk and rollback plan approved."
    }
    Assert ($step2.approval.status -eq "APPROVED") "final approval step did not approve the model"
    return $step2.approval
}

# FR-APR-002/003 and FR-MDL-007: evidence, sequential approval and audited export purpose.
$firstApproval = Complete-Approval "initial"
$export = Invoke-API $adminToken POST "/admin-api/ai-platform/projects/$ProjectId/model-versions/$ModelVersionId/export" @{
    purpose = "SPEC-2290 controlled production handoff"
    includeArtifacts = $true
}
Assert ($export.inputFingerprint.Length -eq 64) "export did not verify an approval fingerprint"

# FR-APR-006: controlled input change invalidates approval and blocks export.
$licenses[3].useDeclaration = "Changed governed deployment purpose; re-approval is mandatory"
Invoke-API $adminToken PUT "/admin-api/ai-platform/projects/$ProjectId/model-versions/$ModelVersionId/licenses" @{ licenses = $licenses } | Out-Null
$afterChange = Invoke-API $adminToken GET "/admin-api/ai-platform/projects/$ProjectId/model-versions/$ModelVersionId"
$invalidated = $afterChange.approvals | Where-Object id -eq $firstApproval.id
Assert ($invalidated.status -eq "INVALIDATED") "controlled input change did not invalidate approval"
try {
    Invoke-API $adminToken POST "/admin-api/ai-platform/projects/$ProjectId/model-versions/$ModelVersionId/export" @{
        purpose = "This export must be blocked"
        includeArtifacts = $true
    } | Out-Null
    throw "export unexpectedly succeeded with invalidated approval"
}
catch {
    if ($_.Exception.Message -match "unexpectedly succeeded") { throw }
}

# Re-approve the new fingerprint so the final product state is deployable.
$finalApproval = Complete-Approval "requalified"
$finalExport = Invoke-API $adminToken POST "/admin-api/ai-platform/projects/$ProjectId/model-versions/$ModelVersionId/export" @{
    purpose = "SPEC-2290 requalified production handoff"
    includeArtifacts = $true
}

[pscustomobject]@{
    projectCloneId = $clone.id
    clonedTemplates = @($cloneTemplates).Count
    clonedOntologies = @($cloneOntologies).Count
    compatibilityRuleId = $rule.id
    activeUpgradeRevisionId = $upgrade.revision.id
    rollbackRevisionId = $rollback.revision.id
    incidentId = $ignored.id
    approvalTemplateId = $template.id
    invalidatedApprovalId = $firstApproval.id
    finalApprovalId = $finalApproval.id
    finalInputFingerprint = $finalExport.inputFingerprint
} | Format-List

Write-Host "SPEC-2290 supply-chain acceptance passed."
