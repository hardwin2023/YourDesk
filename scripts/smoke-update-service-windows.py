#!/usr/bin/env python3
"""隔離執行真正的使用者更新腳本及 .NET 管線；SCM／安裝器使用替身，不提權。"""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import uuid

parser = argparse.ArgumentParser()
parser.add_argument('--shell', default=shutil.which('pwsh') or shutil.which('powershell.exe'))
args = parser.parse_args()
if not args.shell:
    parser.error('需要 pwsh 或 Windows PowerShell')
root = Path(__file__).resolve().parents[1]
rollback = (root/'internal/clientui/update_payload.go').read_text().split('const windowsRollbackScript = `', 1)[1].rsplit('`', 1)[0]
portable = (root/'internal/clientui/update_portable.go').read_text().split('const windowsPortableApplyScript = `', 1)[1].rsplit('`', 1)[0]
script = rollback + portable + (root/'internal/clientui/update_service_script.go').read_text().split('const windowsServiceUpdateScript = windowsRollbackScript + windowsPortableApplyScript + `', 1)[1].rsplit('`', 1)[0]
legacy = (root/'internal/clientui/update_install_windows.go').read_text().split('const windowsUpdateScript = `', 1)[1].rsplit('`', 1)[0].replace('` + windowsRollbackScript + windowsPortableApplyScript + `', rollback + portable)
assert 'RunAs' not in script and 'Start-Service' not in script and 'Stop-Service' not in script

def ps(value):
    return "'" + str(value).replace("'", "''") + "'"

server_source = r'''
param($ticket,$record,$failure)
$ErrorActionPreference='Stop'
$phase='ready'
while ($true) {
 $pipe=New-Object IO.Pipes.NamedPipeServerStream(('YourDeskPrelogin-update-'+$ticket),[IO.Pipes.PipeDirection]::InOut,1,[IO.Pipes.PipeTransmissionMode]::Byte,[IO.Pipes.PipeOptions]::Asynchronous)
 try {
  $pipe.WaitForConnection()
  $encoding=New-Object Text.UTF8Encoding($false)
  $reader=New-Object IO.StreamReader($pipe,$encoding,$false,1024,$true)
  $writer=New-Object IO.StreamWriter($pipe,$encoding,1024,$true)
  $request=$reader.ReadLine() | ConvertFrom-Json
  Add-Content -LiteralPath $record -Value $request.operation
  $errorText=''
  switch ($request.operation) {
   'begin' { $phase='stopped' }
   'commit' { $phase='applied'; if ($failure -eq 'service') { $phase='rolled-back'; $errorText='fixture service health failure' } }
   'rollback' { $phase='rolled-back' }
   'finish' { $phase='complete' }
  }
  $writer.WriteLine((@{phase=$phase;error=$errorText} | ConvertTo-Json -Compress));$writer.Flush()
  $reader.Dispose();$writer.Dispose()
 } finally { $pipe.Dispose() }
}
'''

with tempfile.TemporaryDirectory(prefix='ydp-', dir=None if os.name == 'nt' else '/tmp') as temporary:
    base = Path(temporary)
    env = dict(os.environ, TMPDIR=str(base))  # Unix domain socket 路徑上限；不改產品管線名稱。
    # 使用 PowerShell 自己的 parser 檢查新版／舊版 migration 腳本。
    for name, source in [('service', script), ('legacy', legacy)]:
        candidate = base/(name+'.ps1'); candidate.write_text(source)
        check = "$tokens=$null;$errors=$null;[Management.Automation.Language.Parser]::ParseFile("+ps(candidate)+",[ref]$tokens,[ref]$errors)|Out-Null;if($errors.Count){$errors|Out-String|Write-Error;exit 1}"
        subprocess.run([args.shell,'-NoProfile','-NonInteractive','-Command',check],env=env,check=True,timeout=30)
    server_file=base/'server.ps1';server_file.write_text(server_source)
    for failure in ('', 'installer', 'service', 'launch'):
        folder=base/(failure or 'success');folder.mkdir()
        target=folder/'target';target.mkdir()
        for name in ('YourDesk.exe','yourdesk-client.exe','yourdesk-remote.exe','avcodec-62.dll'):
            (target/name).write_text('old')
        (target/'personal.txt').write_text('preserve')
        (folder/'setup.exe').write_text('fixture')
        ticket=uuid.uuid4().hex
        (folder/'config.json').write_text(json.dumps({'parent':os.getpid(),'target':str(target),'serviceTicket':ticket,'files':['YourDesk.exe','yourdesk-client.exe','yourdesk-remote.exe','avcodec-62.dll','avutil-60.dll']}))
        records,launches=folder/'operations',folder/'launches'
        shim=r'''
$fixtureFailure=FIXTURE_FAILURE
function Get-Process { param($Id,$ErrorAction); $p=New-Object PSObject; $p | Add-Member -MemberType ScriptMethod -Name WaitForExit -Value {param($timeout) return $true}; return $p }
function Start-Process {
 param($FilePath,$ArgumentList,$WorkingDirectory,[switch]$PassThru,[switch]$Wait,$ErrorAction,$Verb)
 if ($Verb) { throw 'Unexpected elevation in ordinary update' }
 if ([IO.Path]::GetFileName($FilePath) -eq 'setup.exe') {
  [IO.File]::WriteAllText((Join-Path $config.target 'YourDesk.exe'),'new')
  [IO.File]::WriteAllText((Join-Path $config.target 'avcodec-62.dll'),'new')
  [IO.File]::WriteAllText((Join-Path $config.target 'avutil-60.dll'),'new')
  return [pscustomobject]@{ExitCode=$(if($fixtureFailure -eq 'installer'){1}else{0})}
 }
 $v=[IO.File]::ReadAllText($FilePath);Add-Content -LiteralPath FIXTURE_LAUNCHES -Value $v
 if ($fixtureFailure -eq 'launch' -and $v -eq 'new') { throw 'fixture launch failed' }
}
Microsoft.PowerShell.Utility\Add-Type -TypeDefinition 'namespace System.Windows { public class MessageBox { public static void Show(string a,string b) {} } }'
function Add-Type { param($AssemblyName) }
'''.replace('FIXTURE_FAILURE',ps(failure)).replace('FIXTURE_LAUNCHES',ps(launches))
        # 記錄放在暫存更新目錄外，成功刪除 staging 後仍可檢查。
        stage=folder/'stage';stage.mkdir()
        for name in ('config.json','setup.exe'): (folder/name).rename(stage/name)
        (stage/'install.ps1').write_text(shim+script)
        server=subprocess.Popen([args.shell,'-NoProfile','-NonInteractive','-File',str(server_file),ticket,str(records),failure],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
        try:
            result=subprocess.run([args.shell,'-NoProfile','-NonInteractive','-File',str(stage/'install.ps1')],env=env,capture_output=True,text=True,timeout=35)
        finally:
            server.terminate();server_out,server_err=server.communicate(timeout=10)
        log=(stage/'update.log').read_text() if (stage/'update.log').exists() else ''
        assert (result.returncode != 0) == bool(failure), (failure,result.returncode,result.stdout,result.stderr,log,server_out,server_err)
        assert (target/'YourDesk.exe').read_text() == ('old' if failure else 'new')
        assert (target/'avcodec-62.dll').read_text() == ('old' if failure else 'new')
        assert (target/'avutil-60.dll').exists() == (not failure)
        assert (target/'personal.txt').read_text() == 'preserve'
        calls=records.read_text().splitlines()
        assert calls[:2] == ['status','begin'], calls
        assert calls[-1] == ('rollback' if failure else 'finish'), calls
        assert launches.read_text().splitlines() == (['new','old'] if failure == 'launch' else ['old'] if failure else ['new'])
        if failure: assert (stage/'update.log').is_file()
        else: assert not stage.exists()
    # 離線 Portable：執行相同的 RunAs 交接／備份／還原腳本，僅替換 OS 權限及 SCM。
    # 使用私有 TMPDIR 中真正的 .NET 管線，回覆舊版也支援的 status=false。
    local_server = base/'local-server.ps1'
    local_server.write_text(r'''
param($serviceRoot,$failure,$record)
$ErrorActionPreference='Stop'
while($true){
 $pipe=New-Object IO.Pipes.NamedPipeServerStream('YourDeskPrelogin-control',[IO.Pipes.PipeDirection]::InOut,1,[IO.Pipes.PipeTransmissionMode]::Byte,[IO.Pipes.PipeOptions]::Asynchronous)
 try{
  $pipe.WaitForConnection();$encoding=New-Object Text.UTF8Encoding($false)
  $reader=New-Object IO.StreamReader($pipe,$encoding,$false,1024,$true)
  $writer=New-Object IO.StreamWriter($pipe,$encoding,1024,$true)
  $request=$reader.ReadLine()|ConvertFrom-Json
  if($request.operation -ne 'status'){throw '離線健康檢查不可要求新版協定或下載'}
  Add-Content -LiteralPath $record -Value $request.operation
  $value=if($failure -eq 'health' -and [IO.File]::ReadAllText((Join-Path $serviceRoot 'yourdesk-client.exe')) -eq 'new'){'"invalid"'}else{'false'}
  $writer.WriteLine($value);$writer.Flush();$reader.Dispose();$writer.Dispose()
 }finally{$pipe.Dispose()}
}
''')
    for failure in ('', 'copy', 'service', 'health', 'launch'):
        folder=base/('offline-'+(failure or 'success'));folder.mkdir()
        target,service_root,stage=folder/'target',folder/'service',folder/'stage'
        for directory in (target,service_root,stage,stage/'payload'):directory.mkdir()
        app_files=['YourDesk.exe','yourdesk-client.exe','yourdesk-remote.exe','avcodec-62.dll','avutil-60.dll','swscale-9.dll']
        service_files=['yourdesk-client.exe','avcodec-62.dll','avutil-60.dll','swscale-9.dll']
        for name in app_files:
            (target/name).write_text('old');(stage/'payload'/name).write_text('new')
        for name in service_files:(service_root/name).write_text('old')
        for name,value in [('config.json','preserve credentials'),('autostart.disabled','runtime-only'),('sas.disabled','sas-off')]:
            (service_root/name).write_text(value)
        (service_root/'updater.protocol').write_text('1')
        (service_root/'desktop-control.v1').write_text('1')
        (target/'personal.txt').write_text('preserve')
        config={'parent':os.getpid(),'target':str(target),'files':app_files,'portable':True,'prelogin':True,'offline':True}
        (stage/'config.json').write_text(json.dumps(config))
        launches,authorization,record=folder/'launches',folder/'authorization',folder/'health'
        # macOS 的 .NET 不支援 WindowsPrincipal；只在測試替身跳過這一段。
        guarded_start=legacy.index(' if ($config.prelogin) {\n  if (-not ([Security.Principal.WindowsPrincipal]')
        guarded_end=legacy.index('  if ($Rollback) {',guarded_start)
        local_script=legacy[:guarded_start]+' if ($config.prelogin) {\n'+legacy[guarded_end:]
        local_script=local_script.replace("$serviceRoot = Join-Path ([Environment]::GetFolderPath('ProgramFiles')) 'YourDeskPrelogin'",'$serviceRoot = '+ps(service_root))
        local_script=local_script.replace('([Security.Principal.WindowsIdentity]::GetCurrent().User.Value)',"'S-1-fixture'")
        local_script=local_script.replace('[DateTime]::UtcNow.AddSeconds(20)','[DateTime]::UtcNow.AddSeconds(2)')
        shims=r'''
$fixtureFailure=FIXTURE_FAILURE
function Get-Process {
 param($Id,$ErrorAction)
 if($ErrorAction -eq 'SilentlyContinue'){return $null}
 $p=New-Object PSObject;$p|Add-Member -MemberType ScriptMethod -Name WaitForExit -Value {param($timeout)return $true};return $p
}
function Start-Service {
 param($Name)
 if($fixtureFailure -eq 'service' -and [IO.File]::ReadAllText((Join-Path FIXTURE_SERVICE 'yourdesk-client.exe')) -eq 'new'){throw 'fixture service failed'}
}
function Stop-Service {param($Name)}
function Get-Service {
 param($Name)
 $p=New-Object PSObject;$p|Add-Member -MemberType ScriptMethod -Name WaitForStatus -Value {param($status,$timeout)};return $p
}
function Copy-Item {
 param($LiteralPath,$Destination,[switch]$Force,[switch]$Recurse,$ErrorAction)
 if($fixtureFailure -eq 'copy' -and $Destination -eq (Join-Path FIXTURE_SERVICE 'swscale-9.dll') -and [IO.File]::ReadAllText($LiteralPath) -eq 'new'){throw 'fixture copy failed'}
 Microsoft.PowerShell.Management\Copy-Item -LiteralPath $LiteralPath -Destination $Destination -Force:$Force -Recurse:$Recurse -ErrorAction Stop
}
function Start-Process {
 param($FilePath,$ArgumentList,$WorkingDirectory,[switch]$PassThru,[switch]$Wait,$ErrorAction,$Verb)
 if($Verb){
  Add-Content -LiteralPath FIXTURE_AUTHORIZATION -Value 'authorized'
  $argsList=@('-NoProfile','-NonInteractive','-File',$PSCommandPath,'-Elevated')
  if($ArgumentList.EndsWith(' -Rollback')){$argsList+='-Rollback'}
  & FIXTURE_SHELL @argsList | Out-Null
  return [pscustomobject]@{ExitCode=$LASTEXITCODE}
 }
 $value=[IO.File]::ReadAllText($FilePath);Add-Content -LiteralPath FIXTURE_LAUNCHES -Value $value
 if($fixtureFailure -eq 'launch' -and $value -eq 'new'){throw 'fixture new app failed'}
}
Microsoft.PowerShell.Utility\Add-Type -TypeDefinition 'namespace System.Windows { public class MessageBox { public static void Show(string a,string b) {} } }'
function Add-Type {param($AssemblyName)}
'''
        for key,value in [('FAILURE',failure),('SERVICE',service_root),('SHELL',args.shell),('AUTHORIZATION',authorization),('LAUNCHES',launches)]:
            shims=shims.replace('FIXTURE_'+key,ps(value))
        # param 必須仍位於腳本第一行，替身置於它後面。
        line,body=local_script.split('\n',1)
        (stage/'install.ps1').write_text(line+'\n'+shims+body)
        server=subprocess.Popen([args.shell,'-NoProfile','-NonInteractive','-File',str(local_server),str(service_root),failure,str(record)],env=env,stdout=subprocess.PIPE,stderr=subprocess.PIPE,text=True)
        try:
            result=subprocess.run([args.shell,'-NoProfile','-NonInteractive','-File',str(stage/'install.ps1')],env=env,capture_output=True,text=True,timeout=35)
        finally:
            server.terminate();server_out,server_err=server.communicate(timeout=10)
        log=(stage/'update.log').read_text() if (stage/'update.log').exists() else ''
        assert (result.returncode != 0)==bool(failure),(failure,result.returncode,result.stdout,result.stderr,log,server_err)
        for directory,names in [(target,app_files),(service_root,service_files)]:
            for name in names:assert (directory/name).read_text()==('old' if failure else 'new'),(failure,directory,name)
        assert (target/'personal.txt').read_text()=='preserve'
        for name,value in [('config.json','preserve credentials'),('autostart.disabled','runtime-only'),('sas.disabled','sas-off')]:
            assert (service_root/name).read_text()==value,(failure,name)
        assert (service_root/'updater.protocol').exists()==bool(failure),'回退／成功後的協定標記不符'
        assert (service_root/'desktop-control.v1').exists()==bool(failure),'回退／成功後的桌面能力標記不符'
        assert authorization.read_text().splitlines()==['authorized']*(2 if failure=='launch' else 1)
        assert launches.read_text().splitlines()==(['new','old'] if failure=='launch' else ['old'] if failure else ['new']),(failure,launches.read_text(),log)
        assert record.read_text().splitlines().count('status')>=3,'必須實際確認控制管線，不只 SCM Running'
        if failure:assert (stage/'update.log').is_file()
        else:assert not stage.exists()
print('PASS: PowerShell 語法、OTA 真實管線交接、離線 ZIP 與舊協定、APP／服務同步替換、設定與模式保留、複製／服務／健康檢查／啟動失敗回復、原帳號重開（未提權或執行 Windows SCM）')
