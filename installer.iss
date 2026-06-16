[Setup]
AppName=Zensu
AppVersion=1.0.0
DefaultDirName={localappdata}\Programs\Zensu
DefaultGroupName=Zensu
UninstallDisplayIcon={app}\zensu.exe
Compression=lzma2
SolidCompression=yes
OutputDir=.
OutputBaseFilename=zensu-setup

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"

[Files]
Source: "build\bin\zensu.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "bin\ffmpeg.exe"; DestDir: "{app}\bin"; Flags: ignoreversion skipifsourcedoesntexist

[Icons]
Name: "{group}\Zensu"; Filename: "{app}\zensu.exe"
Name: "{autodesktop}\Zensu"; Filename: "{app}\zensu.exe"; Tasks: desktopicon

[Registry]
Root: HKCU; Subkey: "Environment"; ValueType: string; ValueName: "Path"; ValueData: "{olddata};{app}"; Flags: preservestringtype; Check: NeedsAddPath(ExpandConstant('{app}'))

[Code]
function NeedsAddPath(Param: string): boolean;
var
  OrigPath: string;
begin
  if not RegQueryStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', OrigPath) then
  begin
    Result := True;
    exit;
  end;
  Result := Pos(Param, OrigPath) = 0;
end;
