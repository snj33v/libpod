package generate

import (
	"testing"
)

func TestValidateRestartPolicy(t *testing.T) {
	type ContainerInfo struct {
		restart string
	}
	tests := []struct {
		name          string
		ContainerInfo ContainerInfo
		wantErr       bool
	}{
		{"good-on", ContainerInfo{restart: "no"}, false},
		{"good-on-success", ContainerInfo{restart: "on-success"}, false},
		{"good-on-failure", ContainerInfo{restart: "on-failure"}, false},
		{"good-on-abnormal", ContainerInfo{restart: "on-abnormal"}, false},
		{"good-on-watchdog", ContainerInfo{restart: "on-watchdog"}, false},
		{"good-on-abort", ContainerInfo{restart: "on-abort"}, false},
		{"good-always", ContainerInfo{restart: "always"}, false},
		{"fail", ContainerInfo{restart: "foobar"}, true},
		{"failblank", ContainerInfo{restart: ""}, true},
	}
	for _, tt := range tests {
		test := tt
		t.Run(tt.name, func(t *testing.T) {
			if err := validateRestartPolicy(test.ContainerInfo.restart); (err != nil) != test.wantErr {
				t.Errorf("ValidateRestartPolicy() error = %v, wantErr %v", err, test.wantErr)
			}
		})
	}
}

func TestCreateContainerSystemdUnit(t *testing.T) {
	goodID := `# container-639c53578af4d84b8800b4635fa4e680ee80fd67e0e6a2d4eea48d1e3230f401.service
# autogenerated by Podman CI

[Unit]
Description=Podman container-639c53578af4d84b8800b4635fa4e680ee80fd67e0e6a2d4eea48d1e3230f401.service
Documentation=man:podman-generate-systemd(1)
Wants=network.target
After=network-online.target

[Service]
Restart=always
ExecStart=/usr/bin/podman start 639c53578af4d84b8800b4635fa4e680ee80fd67e0e6a2d4eea48d1e3230f401
ExecStop=/usr/bin/podman stop -t 10 639c53578af4d84b8800b4635fa4e680ee80fd67e0e6a2d4eea48d1e3230f401
PIDFile=/var/run/containers/storage/overlay-containers/639c53578af4d84b8800b4635fa4e680ee80fd67e0e6a2d4eea48d1e3230f401/userdata/conmon.pid
KillMode=none
Type=forking

[Install]
WantedBy=multi-user.target default.target`

	goodName := `# container-foobar.service
# autogenerated by Podman CI

[Unit]
Description=Podman container-foobar.service
Documentation=man:podman-generate-systemd(1)
Wants=network.target
After=network-online.target

[Service]
Restart=always
ExecStart=/usr/bin/podman start foobar
ExecStop=/usr/bin/podman stop -t 10 foobar
PIDFile=/var/run/containers/storage/overlay-containers/639c53578af4d84b8800b4635fa4e680ee80fd67e0e6a2d4eea48d1e3230f401/userdata/conmon.pid
KillMode=none
Type=forking

[Install]
WantedBy=multi-user.target default.target`

	goodNameBoundTo := `# container-foobar.service
# autogenerated by Podman CI

[Unit]
Description=Podman container-foobar.service
Documentation=man:podman-generate-systemd(1)
Wants=network.target
After=network-online.target
RefuseManualStart=yes
RefuseManualStop=yes
BindsTo=a.service b.service c.service pod.service
After=a.service b.service c.service pod.service

[Service]
Restart=always
ExecStart=/usr/bin/podman start foobar
ExecStop=/usr/bin/podman stop -t 10 foobar
PIDFile=/var/run/containers/storage/overlay-containers/639c53578af4d84b8800b4635fa4e680ee80fd67e0e6a2d4eea48d1e3230f401/userdata/conmon.pid
KillMode=none
Type=forking

[Install]
WantedBy=multi-user.target default.target`

	podGoodName := `# pod-123abc.service
# autogenerated by Podman CI

[Unit]
Description=Podman pod-123abc.service
Documentation=man:podman-generate-systemd(1)
Wants=network.target
After=network-online.target
Requires=container-1.service container-2.service
Before=container-1.service container-2.service

[Service]
Restart=always
ExecStart=/usr/bin/podman start jadda-jadda-infra
ExecStop=/usr/bin/podman stop -t 10 jadda-jadda-infra
PIDFile=/var/run/containers/storage/overlay-containers/639c53578af4d84b8800b4635fa4e680ee80fd67e0e6a2d4eea48d1e3230f401/userdata/conmon.pid
KillMode=none
Type=forking

[Install]
WantedBy=multi-user.target default.target`

	goodNameNew := `# jadda-jadda.service
# autogenerated by Podman CI

[Unit]
Description=Podman jadda-jadda.service
Documentation=man:podman-generate-systemd(1)
Wants=network.target
After=network-online.target

[Service]
Restart=always
ExecStartPre=/usr/bin/rm -f %t/%n-pid %t/%n-cid
ExecStart=/usr/bin/podman run --conmon-pidfile %t/%n-pid --cidfile %t/%n-cid --cgroups=no-conmon --name jadda-jadda --hostname hello-world awesome-image:latest command arg1 ... argN
ExecStop=/usr/bin/podman stop --ignore --cidfile %t/%n-cid -t 42
ExecStopPost=/usr/bin/podman rm --ignore -f --cidfile %t/%n-cid
PIDFile=%t/%n-pid
KillMode=none
Type=forking

[Install]
WantedBy=multi-user.target default.target`

	tests := []struct {
		name    string
		info    ContainerInfo
		want    string
		wantErr bool
	}{

		{"good with id",
			ContainerInfo{
				Executable:    "/usr/bin/podman",
				ServiceName:   "container-639c53578af4d84b8800b4635fa4e680ee80fd67e0e6a2d4eea48d1e3230f401",
				ContainerName: "639c53578af4d84b8800b4635fa4e680ee80fd67e0e6a2d4eea48d1e3230f401",
				RestartPolicy: "always",
				PIDFile:       "/var/run/containers/storage/overlay-containers/639c53578af4d84b8800b4635fa4e680ee80fd67e0e6a2d4eea48d1e3230f401/userdata/conmon.pid",
				StopTimeout:   10,
				PodmanVersion: "CI",
			},
			goodID,
			false,
		},
		{"good with name",
			ContainerInfo{
				Executable:    "/usr/bin/podman",
				ServiceName:   "container-foobar",
				ContainerName: "foobar",
				RestartPolicy: "always",
				PIDFile:       "/var/run/containers/storage/overlay-containers/639c53578af4d84b8800b4635fa4e680ee80fd67e0e6a2d4eea48d1e3230f401/userdata/conmon.pid",
				StopTimeout:   10,
				PodmanVersion: "CI",
			},
			goodName,
			false,
		},
		{"good with name and bound to",
			ContainerInfo{
				Executable:      "/usr/bin/podman",
				ServiceName:     "container-foobar",
				ContainerName:   "foobar",
				RestartPolicy:   "always",
				PIDFile:         "/var/run/containers/storage/overlay-containers/639c53578af4d84b8800b4635fa4e680ee80fd67e0e6a2d4eea48d1e3230f401/userdata/conmon.pid",
				StopTimeout:     10,
				PodmanVersion:   "CI",
				BoundToServices: []string{"pod", "a", "b", "c"},
			},
			goodNameBoundTo,
			false,
		},
		{"pod",
			ContainerInfo{
				Executable:       "/usr/bin/podman",
				ServiceName:      "pod-123abc",
				ContainerName:    "jadda-jadda-infra",
				RestartPolicy:    "always",
				PIDFile:          "/var/run/containers/storage/overlay-containers/639c53578af4d84b8800b4635fa4e680ee80fd67e0e6a2d4eea48d1e3230f401/userdata/conmon.pid",
				StopTimeout:      10,
				PodmanVersion:    "CI",
				RequiredServices: []string{"container-1", "container-2"},
			},
			podGoodName,
			false,
		},
		{"bad restart policy",
			ContainerInfo{
				Executable:    "/usr/bin/podman",
				ServiceName:   "639c53578af4d84b8800b4635fa4e680ee80fd67e0e6a2d4eea48d1e3230f401",
				RestartPolicy: "never",
				PIDFile:       "/var/run/containers/storage/overlay-containers/639c53578af4d84b8800b4635fa4e680ee80fd67e0e6a2d4eea48d1e3230f401/userdata/conmon.pid",
				StopTimeout:   10,
				PodmanVersion: "CI",
			},
			"",
			true,
		},
		{"good with name and generic",
			ContainerInfo{
				Executable:    "/usr/bin/podman",
				ServiceName:   "jadda-jadda",
				ContainerName: "jadda-jadda",
				RestartPolicy: "always",
				PIDFile:       "/var/run/containers/storage/overlay-containers/639c53578af4d84b8800b4635fa4e680ee80fd67e0e6a2d4eea48d1e3230f401/userdata/conmon.pid",
				StopTimeout:   42,
				PodmanVersion: "CI",
				New:           true,
				CreateCommand: []string{"I'll get stripped", "container", "run", "--name", "jadda-jadda", "--hostname", "hello-world", "awesome-image:latest", "command", "arg1", "...", "argN"},
			},
			goodNameNew,
			false,
		},
	}
	for _, tt := range tests {
		test := tt
		t.Run(tt.name, func(t *testing.T) {
			opts := Options{
				Files: false,
				New:   test.info.New,
			}
			got, err := CreateContainerSystemdUnit(&test.info, opts)
			if (err != nil) != test.wantErr {
				t.Errorf("CreateContainerSystemdUnit() error = \n%v, wantErr \n%v", err, test.wantErr)
				return
			}
			if got != test.want {
				t.Errorf("CreateContainerSystemdUnit() = \n%v\n---------> want\n%v", got, test.want)
			}
		})
	}
}
