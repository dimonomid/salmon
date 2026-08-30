[Unit]
Description=Salmon: system health monitor
After=network.target

# Salmon provides monitoring, so a transient failure or unexpected clean exit
# must not leave it permanently stopped. Retry indefinitely, with RestartSec
# below keeping persistent failures from causing a tight restart loop.
StartLimitIntervalSec=0

[Service]
User=salmon
Group=salmon
ExecStart={{ systemdUnitArgument .Executable }} --config {{ systemdUnitArgument .ConfigFilename }}
Restart=always
RestartSec=10

# Hardening
ProtectSystem=full
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
