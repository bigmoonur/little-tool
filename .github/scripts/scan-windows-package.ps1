param(
    [Parameter(Mandatory = $true)][string]$PackageDirectory,
    [Parameter(Mandatory = $true)][string]$ReportDirectory
)

$ErrorActionPreference = 'Stop'
$started = Get-Date
$completed = $false
$scanError = $null
$failure = $null
New-Item -ItemType Directory -Path $ReportDirectory -Force | Out-Null
Start-Transcript -Path (Join-Path $ReportDirectory 'scan-transcript.txt') -Force | Out-Null

try {
    $archives = @(Get-ChildItem -LiteralPath $PackageDirectory -Filter '*.zip' -File)
    if ($archives.Count -ne 1) { throw 'Expected exactly one release ZIP' }
    $archive = $archives[0]
    $expected = (Get-Content -LiteralPath ($archive.FullName + '.sha256') -Raw).Trim().Split(' ')[0]
    if ($expected -notmatch '^[a-fA-F0-9]{64}$' -or
        (Get-FileHash -LiteralPath $archive.FullName -Algorithm SHA256).Hash -ne $expected) {
        throw 'The original release ZIP checksum does not match'
    }
    Expand-Archive -LiteralPath $archive.FullName -DestinationPath (Join-Path $PackageDirectory 'extracted')
    $files = @(Get-ChildItem -LiteralPath $PackageDirectory -Recurse -File | Where-Object { $_.Extension -in '.exe', '.zip' })
    if (@($files | Where-Object Extension -eq '.exe').Count -ne 2) { throw 'Expected portable EXE and installer' }
    $hashes = @($files | ForEach-Object {
        [pscustomobject]@{
            File = $_.FullName.Substring($PackageDirectory.Length + 1)
            SHA256 = (Get-FileHash -LiteralPath $_.FullName -Algorithm SHA256).Hash
            Length = $_.Length
        }
    })
    $hashes | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $ReportDirectory 'package-hashes.json') -Encoding UTF8
    $hashes | Format-Table -AutoSize

    Get-MpComputerStatus | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $ReportDirectory 'status-before.json') -Encoding UTF8
    $preferences = Get-MpPreference
    # GitHub-hosted images exclude C: and D: and disable several scanning features.
    # Remove inherited exclusions on this disposable runner so a clean result is meaningful.
    foreach ($name in @('ExclusionPath', 'ExclusionProcess', 'ExclusionExtension')) {
        $values = @($preferences.$name | Where-Object { $_ })
        if ($values.Count) {
            $arguments = @{}
            $arguments[$name] = $values
            Remove-MpPreference @arguments
        }
    }
    $passivePolicy = 'HKLM:\SOFTWARE\Policies\Microsoft\Windows Advanced Threat Protection'
    if (Test-Path $passivePolicy) {
        Set-ItemProperty -Path $passivePolicy -Name ForceDefenderPassiveMode -Value 0 -Type DWord
    }
    Set-MpPreference -DisableRealtimeMonitoring $false -DisableArchiveScanning $false `
        -DisableIOAVProtection $false -DisableBehaviorMonitoring $false -DisableScriptScanning $false `
        -DisableBlockAtFirstSeen $false -MAPSReporting 2 -SubmitSamplesConsent 2 -PUAProtection 1 `
        -ScanAvgCPULoadFactor 50
    # Automatic sample submission stays disabled; binaries are not uploaded for public analysis.
    Update-MpSignature -UpdateSource MMPC
    $status = Get-MpComputerStatus
    $status | Format-List AMServiceEnabled, AntivirusEnabled, RealTimeProtectionEnabled, AMRunningMode, AMEngineVersion, AntivirusSignatureVersion, AntivirusSignatureLastUpdated
    $status | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $ReportDirectory 'status-scan.json') -Encoding UTF8
    $preferences = Get-MpPreference
    $preferences | Select-Object ExclusionPath, ExclusionProcess, ExclusionExtension, DisableRealtimeMonitoring, DisableArchiveScanning, DisableIOAVProtection, DisableBehaviorMonitoring, DisableScriptScanning, MAPSReporting, SubmitSamplesConsent, PUAProtection | ConvertTo-Json -Depth 4 | Set-Content -LiteralPath (Join-Path $ReportDirectory 'scan-settings.json') -Encoding UTF8
    if (-not $status.AMServiceEnabled -or -not $status.AntivirusEnabled -or -not $status.RealTimeProtectionEnabled) {
        throw 'Defender protection could not be enabled; this is not a clean scan'
    }
    if (@($preferences.ExclusionPath | Where-Object { $_ }).Count -or
        @($preferences.ExclusionProcess | Where-Object { $_ }).Count -or
        @($preferences.ExclusionExtension | Where-Object { $_ }).Count -or $preferences.DisableArchiveScanning) {
        throw 'Inherited exclusions or disabled archive scanning remain; scan cannot be accepted'
    }
    try {
        Start-MpScan -ScanType CustomScan -ScanPath $PackageDirectory
        $completed = $true
    } catch {
        $scanError = $_.ToString()
    }
} catch {
    $failure = $_.ToString()
} finally {
    $threats = @(Get-MpThreat -ErrorAction SilentlyContinue | Select-Object ThreatID, ThreatName, SeverityID, CategoryID, IsActive, DidThreatExecute, Resources)
    $detections = @(Get-MpThreatDetection -ErrorAction SilentlyContinue | Select-Object ThreatID, InitialDetectionTime, LastThreatStatusChangeTime, ActionSuccess, Resources, ProcessName)
    $packageDetections = @($detections | Where-Object {
        @($_.Resources | Where-Object { $_.IndexOf($PackageDirectory, [StringComparison]::OrdinalIgnoreCase) -ge 0 }).Count -gt 0
    })
    $events = @(Get-WinEvent -FilterHashtable @{
        LogName = 'Microsoft-Windows-Windows Defender/Operational'
        StartTime = $started
        Id = @(1000, 1001, 1116, 1117)
    } -ErrorAction SilentlyContinue | Select-Object TimeCreated, Id, Message)
    [pscustomobject]@{
        Started = $started.ToUniversalTime().ToString('o')
        Finished = (Get-Date).ToUniversalTime().ToString('o')
        ScanCompleted = $completed
        ScanError = $scanError
        Failure = $failure
        Threats = $threats
        Detections = $detections
        PackageDetections = $packageDetections
        Events = $events
    } | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath (Join-Path $ReportDirectory 'scan-result.json') -Encoding UTF8
    $threats | Format-List
    $detections | Format-List
    Stop-Transcript | Out-Null
}

if ($failure) { throw $failure }
if ($packageDetections.Count) {
    "Defender detected $($packageDetections.Count) threat record(s) in the original release. See windows-defender-report." >> $env:GITHUB_STEP_SUMMARY
    throw 'Defender detected the release package; keep the original files quarantined pending review'
}
if ($scanError -or -not $completed) { throw "Defender scan did not complete: $scanError" }
"Defender completed a scan of the original release ZIP, installer and portable EXE without detecting a threat on this runner. This does not guarantee identical results on customer devices." >> $env:GITHUB_STEP_SUMMARY
