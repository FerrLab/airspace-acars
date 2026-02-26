[Setup]
AppName=Airspace ACARS
AppVersion=__VERSION__
AppPublisher=Airspace ACARS
DefaultDirName={commonappdata}\Airspace ACARS
DefaultGroupName=Airspace ACARS
UninstallDisplayIcon={app}\Airspace ACARS.exe
OutputDir=bin
OutputBaseFilename=airspace-acars-windows-amd64-setup
SetupIconFile=build\windows\icon.ico
Compression=lzma2
SolidCompression=yes
PrivilegesRequired=lowest
WizardStyle=modern

[Files]
Source: "Airspace ACARS.exe"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\Airspace ACARS"; Filename: "{app}\Airspace ACARS.exe"
Name: "{commondesktop}\Airspace ACARS"; Filename: "{app}\Airspace ACARS.exe"

[Run]
Filename: "{app}\Airspace ACARS.exe"; Description: "Launch Airspace ACARS"; Flags: nowait postinstall skipifsilent
