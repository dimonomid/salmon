[Unit]
Description=Salmon
After=network.target

# Salmon provides monitoring, so a transient or repaired startup failure must
# not leave it permanently stopped. Retry indefinitely, with RestartSec below
# keeping persistent failures from causing a tight restart loop.
StartLimitIntervalSec=0

[Service]
ExecStart={{ systemdUnitArgument .Executable }} --config {{ systemdUnitArgument .ConfigFilename }}
Restart=on-failure
RestartSec=10

# Hardening
ProtectSystem=full
PrivateTmp=true
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
