param(
    [string]$BaseUrl = "http://localhost:58080",
    [string]$CVATUrl = "http://localhost:28080",
    [string]$AdminUsername = "admin",
    [string]$AdminPassword = "admin123",
    [string]$ReviewerUsername = "visionai-reviewer",
    [string]$ReviewerPassword = "VisionAI-Review-2026!",
    [string]$ApproverUsername = "visionai-approver",
    [string]$ApproverPassword = "VisionAI-Approve-2026!",
    [string]$RunTag = (Get-Date -Format "yyyyMMdd-HHmmss")
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
$BaseUrl = $BaseUrl.TrimEnd("/")
$CVATUrl = $CVATUrl.TrimEnd("/")
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
$runtimeRoot = Join-Path $repoRoot "runtime"
$archivePath = Join-Path $runtimeRoot "coco128.zip"
$sourceRoot = Join-Path $runtimeRoot "coco128-source"
$imageRoot = Join-Path $sourceRoot "coco128\images\train2017"
$labelRoot = Join-Path $sourceRoot "coco128\labels\train2017"
$datasetUrl = "https://github.com/ultralytics/assets/releases/download/v0.0.0/coco128.zip"

function Write-Step([string]$Message) {
    Write-Host ("[{0}] {1}" -f (Get-Date -Format "HH:mm:ss"), $Message)
}

function ConvertTo-Utf8JsonBytes($Value) {
    $json = $Value | ConvertTo-Json -Depth 50 -Compress
    return ,([Text.Encoding]::UTF8.GetBytes($json))
}

function Invoke-Login([string]$Username, [string]$Password) {
    $body = ConvertTo-Utf8JsonBytes @{ username = $Username; password = $Password }
    $response = Invoke-RestMethod -Method Post -Uri "$BaseUrl/admin-api/system/auth/login" `
        -Headers @{ "tenant-id" = "1" } -ContentType "application/json; charset=utf-8" -Body $body
    if ($response.code -ne 0 -or -not $response.data.accessToken) {
        throw "Login failed for $Username"
    }
    return $response.data.accessToken
}

function Invoke-VisionAI {
    param(
        [Parameter(Mandatory = $true)][string]$Method,
        [Parameter(Mandatory = $true)][string]$Path,
        $Body = $null,
        [string]$AccessToken = $script:AdminToken,
        [hashtable]$ExtraHeaders = @{}
    )
    $headers = @{ Authorization = "Bearer $AccessToken" }
    foreach ($key in $ExtraHeaders.Keys) {
        $headers[$key] = $ExtraHeaders[$key]
    }
    $arguments = @{
        Method = $Method
        Uri = "$BaseUrl/admin-api$Path"
        Headers = $headers
    }
    if ($null -ne $Body) {
        $arguments.ContentType = "application/json; charset=utf-8"
        $arguments.Body = ConvertTo-Utf8JsonBytes $Body
    }
    $response = Invoke-RestMethod @arguments
    if ($response.code -ne 0) {
        throw "VisionAI API failed: $Method $Path code=$($response.code) msg=$($response.msg)"
    }
    return $response.data
}

function Wait-VisionAIStatus {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][scriptblock]$ReadStatus,
        [Parameter(Mandatory = $true)][string[]]$Success,
        [string[]]$Failure = @("FAILED", "CANCELLED"),
        [int]$TimeoutSeconds = 300
    )
    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    do {
        $data = Invoke-VisionAI -Method Get -Path $Path
        $status = [string](& $ReadStatus $data)
        if ($Success -contains $status) {
            return $data
        }
        if ($Failure -contains $status) {
            throw "Terminal failure '$status' while waiting for $Path"
        }
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw "Timed out after ${TimeoutSeconds}s waiting for $Path"
}

function Wait-CVATMeta {
    param([int64]$TaskId, [hashtable]$Headers, [int]$ExpectedFrames)
    $deadline = (Get-Date).AddMinutes(5)
    do {
        try {
            $meta = Invoke-RestMethod -Method Get -Uri "$CVATUrl/api/tasks/$TaskId/data/meta" -Headers $Headers
            if ($meta.frames.Count -eq $ExpectedFrames) {
                return $meta
            }
        } catch {
            # CVAT returns a transient non-2xx response while media chunks are prepared.
        }
        Start-Sleep -Seconds 2
    } while ((Get-Date) -lt $deadline)
    throw "CVAT task $TaskId did not expose $ExpectedFrames frames"
}

if (-not (Test-Path -LiteralPath $runtimeRoot)) {
    New-Item -ItemType Directory -Path $runtimeRoot | Out-Null
}
if (-not (Test-Path -LiteralPath $archivePath)) {
    Write-Step "Downloading COCO128 from the Ultralytics release"
    Invoke-WebRequest -UseBasicParsing -Uri $datasetUrl -OutFile $archivePath
}
if (-not (Test-Path -LiteralPath $imageRoot)) {
    Write-Step "Extracting COCO128"
    Expand-Archive -LiteralPath $archivePath -DestinationPath $sourceRoot
}
$images = @(Get-ChildItem -LiteralPath $imageRoot -File -Filter "*.jpg")
$labels = @(Get-ChildItem -LiteralPath $labelRoot -File -Filter "*.txt")
if ($images.Count -ne 128 -or $labels.Count -ne 128) {
    throw "COCO128 integrity check failed: images=$($images.Count), labels=$($labels.Count)"
}

Write-Step "Checking VisionAI, CVAT and FiftyOne health"
$health = Invoke-RestMethod "$BaseUrl/health"
$fiftyOneHealth = Invoke-RestMethod "http://localhost:25152/health"
if (-not $health -or $fiftyOneHealth.status -ne "UP" -or $fiftyOneHealth.uiStatus -ne "UP") {
    throw "VisionAI or FiftyOne is not healthy"
}
$script:AdminToken = Invoke-Login $AdminUsername $AdminPassword

Write-Step "Ensuring a separate reviewer account"
$reviewerPage = Invoke-VisionAI -Method Get -Path "/system/user/page?pageNo=1&pageSize=20&username=$ReviewerUsername"
if ([int]$reviewerPage.total -eq 0) {
    $reviewerId = Invoke-VisionAI -Method Post -Path "/system/user/create" -Body @{
        username = $ReviewerUsername
        password = $ReviewerPassword
        nickname = "VisionAI Reviewer"
        status = 0
        deptId = 0
        postIds = @()
        roleIds = @()
    }
} else {
    $reviewerId = [int64]$reviewerPage.list[0].id
    Invoke-VisionAI -Method Put -Path "/system/user/update-password" -Body @{
        id = $reviewerId
        password = $ReviewerPassword
    } | Out-Null
}
$systemRoles = @(Invoke-VisionAI -Method Get -Path "/system/role/simple-list")
$reviewerRoleIds = @(
    $systemRoles |
        Where-Object { $_.code -in @("REVIEWER", "APPROVER", "AUDITOR", "DATA_MANAGER") } |
        ForEach-Object { [int64]$_.id }
)
if ($reviewerRoleIds.Count -ne 4) {
    throw "VisionAI system roles are incomplete; expected REVIEWER, APPROVER, AUDITOR and DATA_MANAGER"
}
Invoke-VisionAI -Method Post -Path "/system/permission/assign-user-role" -Body @{
    userId = $reviewerId
    roleIds = $reviewerRoleIds
} | Out-Null
$approverPage = Invoke-VisionAI -Method Get -Path "/system/user/page?pageNo=1&pageSize=20&username=$ApproverUsername"
if ([int]$approverPage.total -eq 0) {
    $approverId = Invoke-VisionAI -Method Post -Path "/system/user/create" -Body @{
        username = $ApproverUsername
        password = $ApproverPassword
        nickname = "VisionAI Production Approver"
        status = 0
        deptId = 0
        postIds = @()
        roleIds = @()
    }
} else {
    $approverId = [int64]$approverPage.list[0].id
    Invoke-VisionAI -Method Put -Path "/system/user/update-password" -Body @{
        id = $approverId
        password = $ApproverPassword
    } | Out-Null
}
$approverRoleId = [int64](
    $systemRoles |
        Where-Object { $_.code -eq "APPROVER" } |
        Select-Object -First 1
).id
if ($approverRoleId -le 0) {
    throw "VisionAI APPROVER system role is missing"
}
Invoke-VisionAI -Method Post -Path "/system/permission/assign-user-role" -Body @{
    userId = $approverId
    roleIds = @($approverRoleId)
} | Out-Null

Write-Step "Creating the COCO128 acceptance project"
$project = Invoke-VisionAI -Method Post -Path "/ai-platform/projects" -Body @{
    code = "COCO128-$RunTag"
    name = "COCO128 Object Detection"
    description = "Public COCO128 dataset acceptance: data, annotation, training, evaluation, approval, deployment and feedback."
}
$projectId = [int64]$project.id
Invoke-VisionAI -Method Put -Path "/ai-platform/projects/$projectId/status" -Body @{ status = "ACTIVE" } | Out-Null
Invoke-VisionAI -Method Put -Path "/ai-platform/projects/$projectId/members" -Body @{
    userId = $reviewerId
} | Out-Null
Invoke-VisionAI -Method Put -Path "/ai-platform/projects/$projectId/members" -Body @{
    userId = $approverId
} | Out-Null
Invoke-VisionAI -Method Put -Path "/ai-platform/cvat-user-mappings" -Body @{
    platformUserId = 1
    cvatUserId = 1
    cvatUsername = "visionai"
} | Out-Null

Write-Step "Copying 128 public images into the governed import root"
$containerSource = "coco128-$RunTag"
& docker exec visionai-backend mkdir -p "/imports/$containerSource"
if ($LASTEXITCODE -ne 0) { throw "Unable to prepare the import directory" }
& docker cp "$imageRoot\." "visionai-backend:/imports/$containerSource"
if ($LASTEXITCODE -ne 0) { throw "Unable to copy COCO128 images into the import directory" }

Write-Step "Importing COCO128 assets"
$import = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/asset-imports" -Body @{
    sourceType = "DIRECTORY"
    source = $containerSource
    duplicatePolicy = "REFERENCE"
    options = @{ dataset = "COCO128"; sourceUrl = $datasetUrl }
} -ExtraHeaders @{ "Idempotency-Key" = "coco128-import-$RunTag" }
$importId = [int64]$import.importRun.id
$importFinal = Wait-VisionAIStatus -Path "/ai-platform/projects/$projectId/asset-imports/$importId" `
    -ReadStatus { param($data) $data.importRun.status } -Success @("SUCCEEDED") -TimeoutSeconds 300
$assetPage1 = Invoke-VisionAI -Method Get -Path "/ai-platform/projects/$projectId/assets?pageNo=1&pageSize=100&status=READY"
$assetPage2 = Invoke-VisionAI -Method Get -Path "/ai-platform/projects/$projectId/assets?pageNo=2&pageSize=100&status=READY"
$assetRows = @(@($assetPage1.list) + @($assetPage2.list) | Sort-Object id)
if ($assetRows.Count -ne 128) {
    throw "Asset import completed but expected 128 READY assets, got $($assetRows.Count)"
}
$assetIds = @($assetRows | ForEach-Object { [int64]$_.id })

Write-Step "Applying governed business, scene and source tags to all COCO128 assets"
$tagDefinitions = @(
    @{ code = "benchmark"; name = "Public benchmark"; category = "BUSINESS"; color = "#2563eb"; description = "Public benchmark data used for repeatable acceptance"; enabled = $true },
    @{ code = "daylight"; name = "Mixed daylight scenes"; category = "SCENE"; color = "#16a34a"; description = "COCO daylight and mixed-scene imagery"; enabled = $true },
    @{ code = "ultralytics-coco128"; name = "Ultralytics COCO128"; category = "SOURCE"; color = "#ea580c"; description = "Ultralytics public COCO128 release"; enabled = $true }
)
$tagDefinitionIds = @()
foreach ($definition in $tagDefinitions) {
    $created = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/tag-definitions" -Body $definition
    $tagDefinitionIds += [int64]$created.id
}
Invoke-VisionAI -Method Put -Path "/ai-platform/projects/$projectId/assets/tags" -Body @{
    assetIds = $assetIds
    definitionIds = $tagDefinitionIds
    mode = "ADD"
} | Out-Null

Write-Step "Freezing a 128-image asset collection"
$collection = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/collections" -Body @{
    name = "COCO128 Public Images"
    description = "First 128 images from COCO train2017, used for public pipeline acceptance."
    filter = @{ source = "COCO128"; imageCount = 128; tagDefinitionIds = $tagDefinitionIds }
}
$collectionId = [int64]$collection.id
Invoke-VisionAI -Method Put -Path "/ai-platform/projects/$projectId/collections/$collectionId/assets" -Body @{ assetIds = $assetIds } | Out-Null
Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/collections/$collectionId/freeze" | Out-Null

$classNames = @(
    "person", "bicycle", "car", "motorcycle", "airplane", "bus", "train", "truck", "boat", "traffic light",
    "fire hydrant", "stop sign", "parking meter", "bench", "bird", "cat", "dog", "horse", "sheep", "cow",
    "elephant", "bear", "zebra", "giraffe", "backpack", "umbrella", "handbag", "tie", "suitcase", "frisbee",
    "skis", "snowboard", "sports ball", "kite", "baseball bat", "baseball glove", "skateboard", "surfboard", "tennis racket", "bottle",
    "wine glass", "cup", "fork", "knife", "spoon", "bowl", "banana", "apple", "sandwich", "orange",
    "broccoli", "carrot", "hot dog", "pizza", "donut", "cake", "chair", "couch", "potted plant", "bed",
    "dining table", "toilet", "tv", "laptop", "mouse", "remote", "keyboard", "cell phone", "microwave", "oven",
    "toaster", "sink", "refrigerator", "book", "clock", "vase", "scissors", "teddy bear", "hair drier", "toothbrush"
)
$colors = @("#ef4444", "#f97316", "#eab308", "#22c55e", "#14b8a6", "#06b6d4", "#3b82f6", "#6366f1", "#a855f7", "#ec4899")
$annotationLabels = for ($index = 0; $index -lt $classNames.Count; $index++) {
    @{
        code = "coco_$($index.ToString('00'))"
        name = $classNames[$index]
        color = $colors[$index % $colors.Count]
        shapeType = "rectangle"
        sort = $index
        attributes = @()
    }
}

Write-Step "Creating and publishing the governed COCO 80 ontology"
$ontologyResult = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/ontologies" -Body @{
    code = "coco_detection"
    name = "COCO 2017 Detection"
    taskType = "CV_DETECTION"
    description = "The 80-category COCO 2017 detection ontology used by the public acceptance pipeline."
}
$ontologyId = [int64]$ontologyResult.ontology.id
$ontologyVersionId = [int64]$ontologyResult.version.id
Invoke-VisionAI -Method Put -Path "/ai-platform/projects/$projectId/ontology-versions/$ontologyVersionId/labels" -Body @{
    labels = $annotationLabels
} | Out-Null
$publishedOntology = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/ontology-versions/$ontologyVersionId/publish"
if (-not $publishedOntology.checksum -or $publishedOntology.status -ne "PUBLISHED") {
    throw "COCO ontology publishing failed"
}

Write-Step "Creating a governed CVAT task with the 80-class COCO ontology"
$annotationTask = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/annotation-tasks" -Body @{
    name = "COCO128 Ground Truth Review"
    taskType = "CV_DETECTION"
    collectionId = $collectionId
    tagDefinitionIds = $tagDefinitionIds
    ontologyVersionId = $ontologyVersionId
    annotatorIds = @(1)
    reviewerIds = @(1)
}
$annotationTaskId = [int64]$annotationTask.id
Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/annotation-tasks/$annotationTaskId/prepare" | Out-Null
$annotationReady = Wait-VisionAIStatus -Path "/ai-platform/projects/$projectId/annotation-tasks/$annotationTaskId" `
    -ReadStatus { param($data) $data.task.status } -Success @("READY") -TimeoutSeconds 300
$cvatTaskId = [int64]$annotationReady.binding.externalId

Write-Step "Loading the official YOLO labels into CVAT task $cvatTaskId"
$basic = [Convert]::ToBase64String([Text.Encoding]::ASCII.GetBytes("visionai:visionai_cvat_dev"))
$cvatHeaders = @{
    Authorization = "Basic $basic"
    Accept = "application/vnd.cvat+json"
}
$cvatLabels = Invoke-RestMethod -Method Get -Uri "$CVATUrl/api/labels?task_id=$cvatTaskId&page_size=100" -Headers $cvatHeaders
$cvatMeta = Wait-CVATMeta -TaskId $cvatTaskId -Headers $cvatHeaders -ExpectedFrames 128
$labelIdByName = @{}
foreach ($item in $cvatLabels.results) {
    $labelIdByName[[string]$item.name] = [int64]$item.id
}
if ($labelIdByName.Count -ne 80) {
    throw "Expected 80 CVAT labels, got $($labelIdByName.Count)"
}
$culture = [Globalization.CultureInfo]::InvariantCulture
$shapes = [Collections.Generic.List[object]]::new()
for ($frameIndex = 0; $frameIndex -lt $cvatMeta.frames.Count; $frameIndex++) {
    $frame = $cvatMeta.frames[$frameIndex]
    $filename = Split-Path -Leaf ([string]$frame.name)
    $labelPath = Join-Path $labelRoot (([IO.Path]::GetFileNameWithoutExtension($filename)) + ".txt")
    if (-not (Test-Path -LiteralPath $labelPath)) {
        continue
    }
    foreach ($line in (Get-Content -LiteralPath $labelPath)) {
        if ([string]::IsNullOrWhiteSpace($line)) { continue }
        $parts = $line.Trim() -split "\s+"
        $classIndex = [int]$parts[0]
        $xCenter = [double]::Parse($parts[1], $culture)
        $yCenter = [double]::Parse($parts[2], $culture)
        $boxWidth = [double]::Parse($parts[3], $culture)
        $boxHeight = [double]::Parse($parts[4], $culture)
        $x1 = [Math]::Max(0.0, ($xCenter - $boxWidth / 2.0) * [double]$frame.width)
        $y1 = [Math]::Max(0.0, ($yCenter - $boxHeight / 2.0) * [double]$frame.height)
        $x2 = [Math]::Min([double]$frame.width, ($xCenter + $boxWidth / 2.0) * [double]$frame.width)
        $y2 = [Math]::Min([double]$frame.height, ($yCenter + $boxHeight / 2.0) * [double]$frame.height)
        $shapes.Add([ordered]@{
            type = "rectangle"
            frame = $frameIndex
            label_id = $labelIdByName[$classNames[$classIndex]]
            group = 0
            source = "manual"
            occluded = $false
            outside = $false
            z_order = 0
            rotation = 0.0
            points = @($x1, $y1, $x2, $y2)
            attributes = @()
        })
    }
}
if ($shapes.Count -lt 100) {
    throw "Unexpectedly few COCO annotations: $($shapes.Count)"
}
$annotationPayload = ConvertTo-Utf8JsonBytes @{
    version = 0
    tags = @()
    shapes = @($shapes)
    tracks = @()
}
try {
    Invoke-RestMethod -Method Put -Uri "$CVATUrl/api/tasks/$cvatTaskId/annotations" -Headers $cvatHeaders `
        -ContentType "application/json; charset=utf-8" -Body $annotationPayload | Out-Null
}
catch {
    $response = $_.Exception.Response
    if ($null -ne $response) {
        $reader = [IO.StreamReader]::new($response.GetResponseStream())
        $detail = $reader.ReadToEnd()
        $reader.Dispose()
        throw "CVAT annotation import failed: $detail"
    }
    throw
}

Write-Step "Reviewing and exporting the immutable annotation revision"
Invoke-VisionAI -Method Put -Path "/ai-platform/projects/$projectId/annotation-tasks/$annotationTaskId/status" -Body @{ status = "ANNOTATING" } | Out-Null
Invoke-VisionAI -Method Put -Path "/ai-platform/projects/$projectId/annotation-tasks/$annotationTaskId/status" -Body @{ status = "REVIEWING" } | Out-Null
Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/annotation-tasks/$annotationTaskId/review" -Body @{
    decision = "APPROVE"
    code = ""
    reason = "COCO128 labels loaded from the public YOLO annotation files."
} | Out-Null
Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/annotation-tasks/$annotationTaskId/export" | Out-Null
$annotationClosed = Wait-VisionAIStatus -Path "/ai-platform/projects/$projectId/annotation-tasks/$annotationTaskId" `
    -ReadStatus { param($data) $data.task.status } -Success @("CLOSED") -TimeoutSeconds 300
$annotationRevisionId = [int64]$annotationClosed.revisions[0].id
$annotationCount = [int64]$annotationClosed.revisions[0].annotationCount

Write-Step "Creating, validating and freezing the COCO128 dataset version"
$dataset = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/datasets" -Body @{
    name = "COCO128 Detection Dataset"
    taskType = "CV_DETECTION"
    description = "Public COCO128 images and imported COCO ground-truth boxes."
    ownerUserId = 1
}
$datasetId = [int64]$dataset.id
$datasetVersion = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/datasets/$datasetId/versions" -Body @{
    sourceType = "COLLECTION"
    sourceId = $collectionId
    annotationRevisionId = $annotationRevisionId
    ontologyVersionId = $ontologyVersionId
    splitSeed = 20260724
    split = @{ TRAIN = 0.8; VAL = 0.1; TEST = 0.1 }
}
$datasetVersionId = [int64]$datasetVersion.id
Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/dataset-versions/$datasetVersionId/validate" | Out-Null
$datasetReady = Wait-VisionAIStatus -Path "/ai-platform/projects/$projectId/dataset-versions/$datasetVersionId" `
    -ReadStatus { param($data) $data.version.status } -Success @("READY") -Failure @("DRAFT", "FAILED") -TimeoutSeconds 300
Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/dataset-versions/$datasetVersionId/freeze" | Out-Null
$datasetFrozen = Wait-VisionAIStatus -Path "/ai-platform/projects/$projectId/dataset-versions/$datasetVersionId" `
    -ReadStatus { param($data) $data.version.status } -Success @("FROZEN") -Failure @("DRAFT", "FAILED") -TimeoutSeconds 300

Write-Step "Registering the RTX 3060 node and running the LocalDocker CUDA contract"
$gpuModel = (& nvidia-smi --query-gpu=name --format=csv,noheader | Select-Object -First 1).Trim()
$gpuMemoryMiB = [int]((& nvidia-smi --query-gpu=memory.total --format=csv,noheader,nounits | Select-Object -First 1).Trim())
$driverVersion = (& nvidia-smi --query-gpu=driver_version --format=csv,noheader | Select-Object -First 1).Trim()
if (-not $gpuModel) {
    throw "No NVIDIA GPU was detected for the required GPU acceptance run"
}
Invoke-VisionAI -Method Post -Path "/ai-platform/resources/nodes/heartbeat" -Body @{
    nodeKey = "windows-rtx3060"
    name = "Windows RTX 3060 workstation"
    gpuModel = $gpuModel
    gpuCount = 1
    gpuMemoryBytes = [int64]$gpuMemoryMiB * 1MB
    gpuUsedBytes = 0
    driverVersion = $driverVersion
    cudaVersion = "12.6"
    labels = @{ os = "windows"; runtime = "docker-desktop"; acceptance = "COCO128" }
} | Out-Null
Invoke-VisionAI -Method Put -Path "/ai-platform/resources/queues" -Body @{
    name = "gpu-local"
    provider = "LOCAL_DOCKER"
    externalQueue = "gpu-local"
    priority = 100
    enabled = $true
} | Out-Null
$trainerDigest = (docker image inspect visionai/trainer-gpu:cuda12.6 --format '{{.Id}}').Trim()
if (-not $trainerDigest.StartsWith("sha256:")) {
    throw "Immutable GPU trainer image ID is unavailable"
}
$template = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/training-templates" -Body @{
    name = "COCO128 RTX 3060 CUDA Trainer"
    aiType = "CV_DETECTION"
    description = "Immutable CUDA trainer that reads staged COCO128 images and records GPU evidence."
}
$templateId = [int64]$template.id
$templateVersion = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/training-templates/$templateId/versions" -Body @{
    trainer = "TorchVisionFasterRCNN"
    imageRef = $trainerDigest
    entrypoint = ""
    parameterSchema = @{ type = "object"; properties = @{ epochs = @{ type = "integer"; minimum = 1 }; batchSize = @{ type = "integer"; minimum = 1 }; learningRate = @{ type = "number"; exclusiveMinimum = 0 }; pretrained = @{ type = "boolean" } } }
    outputProtocol = "visionai.result-manifest.v1"
    resourceRequirements = @{ cpu = 2; memoryBytes = 4294967296; gpuMin = 1; gpuMax = 1 }
    compatibility = @{
        datasetTypes = @("CV_DETECTION")
        modelTypes = @("CV_DETECTION")
        providers = @("LOCAL_DOCKER", "CLEARML")
        cudaRange = ">=12.0 <13.0"
        driverRange = ">=550"
        minCUDA = "12.0"
        minDriver = "550"
        providerVersions = @{
            LOCAL_DOCKER = "Docker Engine 27+"
            CLEARML = "2.x"
        }
    }
    licensePolicy = @{ allowed = $true; dataset = "COCO128" }
}
$templateVersionId = [int64]$templateVersion.id
Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/training-template-versions/$templateVersionId/smoke" | Out-Null
Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/training-template-versions/$templateVersionId/publish" | Out-Null
$training = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/training-runs" -Body @{
    name = "COCO128 RTX 3060 GPU Training"
    datasetVersionId = $datasetVersionId
    templateVersionId = $templateVersionId
    provider = "LOCAL_DOCKER"
    queue = "gpu-local"
    gpuCount = 1
    priority = 50
    parameters = @{ epochs = 2; batchSize = 2; learningRate = 0.0025; pretrained = $true; seed = 20260727; dataset = "COCO128"; imageSize = 320 }
    runtimeSpec = @{ memoryBytes = 4294967296; cpus = 2 }
    codeCommit = "coco128-rtx3060-acceptance"
    pretrainedRef = "torchvision://fasterrcnn_mobilenet_v3_large_320_fpn/COCO_V1"
} -ExtraHeaders @{ "Idempotency-Key" = "coco128-training-$RunTag" }
$trainingRunId = [int64]$training.run.id
$trainingFinal = Wait-VisionAIStatus -Path "/ai-platform/projects/$projectId/training-runs/$trainingRunId" `
    -ReadStatus { param($data) $data.run.status } -Success @("SUCCEEDED") -TimeoutSeconds 600

Write-Step "Evaluating the training run and syncing samples to FiftyOne"
$suite = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/evaluation-suites" -Body @{
    name = "COCO128 Release Gate"
    datasetVersionId = $datasetVersionId
    slices = @("all", "small-object", "occluded", "low-confidence")
    thresholds = @{ mAP = 0.5; precision = 0.25; recall = 0.4 }
    gatePolicy = "MUST_PASS"
}
$suiteId = [int64]$suite.id
$evaluation = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/evaluation-suites/$suiteId/runs" -Body @{
    trainingRunId = $trainingRunId
    baselineRunId = 0
}
$evaluationRunId = [int64]$evaluation.run.id
$evaluationFinal = Wait-VisionAIStatus -Path "/ai-platform/projects/$projectId/evaluation-runs/$evaluationRunId" `
    -ReadStatus { param($data) $data.run.status } -Success @("SUCCEEDED") -TimeoutSeconds 600
if ($evaluationFinal.run.gateDecision -ne "PASSED") {
    throw "Evaluation completed but gate decision is $($evaluationFinal.run.gateDecision)"
}
Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/evaluation-runs/$evaluationRunId/workbench" | Out-Null

Write-Step "Registering the model and enforcing four-eyes approval"
$modelVersion = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/models/register" -Body @{
    name = "COCO128 Baseline Detector"
    description = "COCO128 public dataset lifecycle model."
    trainingRunId = $trainingRunId
    evaluationRunId = $evaluationRunId
    semanticVersion = "1.0.0"
}
$modelVersionId = [int64]$modelVersion.id
$approval = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/model-versions/$modelVersionId/approvals" -Body @{
    approvalType = "PRODUCTION_QUALIFICATION"
    targetEnvironment = "PRODUCTION"
    riskSummary = "COCO128 may miss small, occluded or low-confidence objects; gate results and failure samples are immutable."
    rollbackPlan = "Stop the active revision and restore the previous approved model while preserving inference traces and audit evidence."
}
$approvalId = [int64]$approval.id
$reviewerToken = Invoke-Login $ReviewerUsername $ReviewerPassword
Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/approvals/$approvalId/decision" -AccessToken $reviewerToken -Body @{
    decision = "APPROVE"
    comment = "COCO128 evidence, lineage, quality gate and immutable artifacts verified."
} | Out-Null
$approverToken = Invoke-Login $ApproverUsername $ApproverPassword
Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/approvals/$approvalId/decision" -AccessToken $approverToken -Body @{
    decision = "APPROVE"
    comment = "Production risk, rollback plan and release boundary verified."
} | Out-Null

Write-Step "Deploying, running inference and capturing production feedback"
$deployment = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/deployments" -Body @{
    name = "COCO128 Production Endpoint"
    environment = "PRODUCTION"
    modelVersionId = $modelVersionId
    config = @{ confidenceThreshold = 0.95; replicas = 1 }
    allowUnapprovedTest = $false
}
$deploymentId = [int64]$deployment.deployment.id
$deploymentFinal = Wait-VisionAIStatus -Path "/ai-platform/projects/$projectId/deployments/$deploymentId" `
    -ReadStatus { param($data) $data.deployment.status } -Success @("RUNNING") -TimeoutSeconds 300
Invoke-VisionAI -Method Put -Path "/ai-platform/projects/$projectId/feedback-policy" -Body @{
    enabled = $true
    randomRate = 0.0
    confidenceBelow = 1.0
    captureEmpty = $true
    captureErrors = $true
    dailyLimit = 100
    retentionDays = 30
    redactionPolicy = @{ storeRequestBody = $false; preserveAssetReference = $true }
    sensitiveReview = $true
} | Out-Null
foreach ($assetId in $assetIds[0..2]) {
    Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/deployments/$deploymentId/predict" -Body @{ assetId = $assetId } | Out-Null
}
$feedbackSamples = @(Invoke-VisionAI -Method Get -Path "/ai-platform/projects/$projectId/feedback-samples?status=PENDING")
if ($feedbackSamples.Count -lt 1) {
    throw "Inference succeeded but no feedback sample was captured"
}
$feedbackBatch = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/feedback-batches" -Body @{
    name = "COCO128 Production Hard Samples"
    sampleIds = @($feedbackSamples | ForEach-Object { [int64]$_.id })
}
$feedbackBatchId = [int64]$feedbackBatch.id
Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/feedback-batches/$feedbackBatchId/review" -AccessToken $reviewerToken -Body @{
    decision = "PRIVACY_APPROVE"
    comment = "Redaction, retention and restricted-use evidence verified by a reviewer separate from the batch creator."
} | Out-Null
$feedbackReview = Invoke-VisionAI -Method Post -Path "/ai-platform/projects/$projectId/feedback-batches/$feedbackBatchId/review" -Body @{
    decision = "ACCEPT"
    comment = "Accepted for governed relabeling and the next dataset revision."
}
$feedbackTaskId = [int64]$feedbackReview.annotationTask.id
$feedbackFinal = Wait-VisionAIStatus -Path "/ai-platform/projects/$projectId/annotation-tasks/$feedbackTaskId" `
    -ReadStatus { param($data) $data.task.status } -Success @("READY") -TimeoutSeconds 300

$result = [ordered]@{
    dataset = [ordered]@{
        name = "COCO128"
        sourceUrl = $datasetUrl
        archiveSha256 = (Get-FileHash -Algorithm SHA256 $archivePath).Hash.ToLowerInvariant()
        images = $images.Count
        yoloLabelFiles = $labels.Count
        cvatShapes = $shapes.Count
    }
    projectId = $projectId
    assetImportId = $importId
    assetCount = $assetRows.Count
    collectionId = $collectionId
    annotationTaskId = $annotationTaskId
    cvatTaskId = $cvatTaskId
    annotationRevisionId = $annotationRevisionId
    annotationCount = $annotationCount
    datasetId = $datasetId
    datasetVersionId = $datasetVersionId
    datasetStatus = $datasetFrozen.version.status
    trainingRunId = $trainingRunId
    trainingStatus = $trainingFinal.run.status
    evaluationRunId = $evaluationRunId
    evaluationStatus = $evaluationFinal.run.status
    gateDecision = $evaluationFinal.run.gateDecision
    modelVersionId = $modelVersionId
    approvalId = $approvalId
    deploymentId = $deploymentId
    deploymentStatus = $deploymentFinal.deployment.status
    feedbackBatchId = $feedbackBatchId
    feedbackAnnotationTaskId = $feedbackTaskId
    feedbackStatus = $feedbackFinal.task.status
    completedAt = (Get-Date).ToUniversalTime().ToString("o")
}
$resultPath = Join-Path $runtimeRoot "coco128-acceptance-$RunTag.json"
$result | ConvertTo-Json -Depth 10 | Set-Content -LiteralPath $resultPath -Encoding UTF8

Write-Step "COCO128 lifecycle acceptance passed"
$result | ConvertTo-Json -Depth 10
Write-Host "Evidence: $resultPath"
