[Desktop Entry]
Type=Application
Name=Salmon Watch
Comment=Show Salmon status in the desktop tray
Exec={{ desktopExecArgument .Executable }} --config {{ desktopExecArgument .ConfigFilename }}
Terminal=false
