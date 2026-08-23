[Unit]
Description=Salmon
After=network.target
StartLimitIntervalSec=60
StartLimitBurst=4

[Service]
ExecStart={{ systemdUnitArgument .Executable }} --config {{ systemdUnitArgument .ConfigFilename }}
Restart=on-failure
RestartSec=1

# Hardening
ProtectSystem=full
PrivateTmp=true
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
