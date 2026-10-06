<!--
SPDX-FileCopyrightText: 2026 Ci4Rail GmbH

SPDX-License-Identifier: Apache-2.0
-->

# Mocks overview

We need mocks for all interfaces used by the moducop-core-api-server. This includes:
- Filesystem operations 
- mender-update command
- os-customization-set command
- reboot command
- docker command

All mock commands should be implemented as Go CLI commands.

## Filesystem operations

The moducop-core-api-server will run in a chroot environment, the mock shall simulate the following folders:
- /etc/issue - mirror the /etc/issue file of the current rootfs
- /data/mender-app/{application-name]/manifests - contains the manifest folder of mender artifacts for application updates. Contains docker-compose.yaml

## Mocks interaction

The mock commands and the provided filesystem shall interact with each other to simulate the behavior of the actual system. For example, when mender-update install is called, it should write the new rootfs to a temporary directory and update the /etc/issue file to reflect the new rootfs. When reboot is called, it should switch the pointer to the current rootfs to the new rootfs if it was installed but not committed, or switch back to the old rootfs if reboot is called with an installed but not committed update.

Also the "mender-update install" command, when called with an application update artifact, should write the docker-compose.yaml to /data/mender-app/{application-name]/manifests/docker-compose.yaml, and the mocked docker command should show running containers based on the docker-compose.yaml file in that directory.

## mender-update command

mender-update is used for
- rootfs updates
- application updates
- Core OS Customization Set updates

They are NOT independent, they share internal state. While one update is installed but not committed, no other update can be installed. If a rootfs update is installed but not committed, application updates cannot be installed, and vice versa.

### Mender version profiles

Set `MOCK_MENDER_VERSION=4` or `MOCK_MENDER_VERSION=5` in the environment
inherited by the mock commands. Unset or empty selects `4`, preserving the
existing mock behavior. Other values are rejected before accessing mock state.
The setting is read on each invocation and is not persisted in `state.json`.

```sh
MOCK_MENDER_VERSION=4 mocks/bin/mender-update install update.mender
MOCK_MENDER_VERSION=5 mocks/bin/mender-update install update.mender
```

`mender-update --version` reports `4.0.5` or `5.1.0` for the selected profile.
The Mender 4 output examples below describe the default profile. Mender 5 mode
uses the same rootfs, application, customization and reboot simulation, with
these differences for the existing commands:

- Failure summaries print the operation and disposition on separate lines.
  Opening/parsing an artifact uses `Streaming failed.`; installation uses
  `Installation failed.`; commit uses `Committing failed.`.
- Successful rollback after a failed commit prints `Rolled back.` instead of
  `Rolled back modifications.`.
- Diagnostics and `Could not fulfill request:` messages go to stderr.
- Refusing an install because an update is pending prints the pending-update
  diagnostic without an installation-failure summary.
- Commit or rollback without an update prints `No update in progress.` and
  exits with code 2. The default profile retains the legacy mock behavior
  (`Nothing to commit.` / `Rolled back.`, exit 0).
- `commit` rejects an interrupted installation with `Cannot commit from this
  state.` on stderr and exit code 1, without changing the saved transaction.
  Completed installations awaiting commit remain committable.
- `resume` continues an interrupted installation using its saved artifact path
  and checkpoint, or commits a transaction already awaiting commit. With no
  pending transaction it exits with code 2. Mender 4 mode rejects `resume`.

The Mender 5 profile persists `install_phase` and `resume_artifact` in
`state.json`. Application checkpoints distinguish stopped containers, renamed
old files, and extracted new files; resuming skips completed steps and finishes
with the application's regular automatic commit. Rootfs and customization
installation checkpoints can rerun installation, which finishes awaiting
commit. Error injection remains configured until explicitly cleared.

Legacy state without a phase is treated as awaiting commit for rootfs and
customization. Legacy interrupted application state is rejected by `commit`;
without a saved artifact path it cannot be resumed and must be rolled back.

Successful install and commit messages are shared by both profiles. These are
profiles of the mock's existing command set, not complete Mender clients:
`--stop-before`, `--reboot-exit-code`, signature enforcement, and
post-commit/cleanup error injection are not simulated yet.

Verify both profiles with `go test ./mocks/...` from the repository root.

### mender-update for Rootfs

shall simulate rootfs A/B update.

mender-update install <image-file>
mender-update commit
mender-update rollback

<file>.mender is a tar file containing

version
manifest
header.tar.gz or header.tar
data/0000.tar.gz

Inside header.tar.gz or header.tar:

header-info
headers/0000/type-info
headers/0000/meta-data

Read header-info.

For rootfs update, it looks like this:
{"payloads":[{"type":"rootfs-image"}],"artifact_provides":{"artifact_name":"cpu01-standard-v2.7.0.40ee657.20260218.1208"},"artifact_depends":{"device_type":["moducop-cpu01"]}}

If type is rootfs-image, then we need to extract data/0000.tar.gz and write it to a temporary directory. It should simulate an A/B 
update. So persist a pointer to the current rootfs, and a simulated reboot will switch the pointer to the new rootfs.

Write shall take at least 10 seconds to complete, and print progress every second.

When finished, write

```
Installed, but not committed.
Use 'commit' to update, or 'rollback' to roll back the update.
```
exit with code 0.

If device_type is not "moducop-cpu01", print
```
record_id=1 severity=error time="2026-Mar-03 07:43:32.990506" name="Global" msg="Artifact device type doesn't match"
Installation failed. System not modified.
```
exit with code 1.

If file does not exist, print
```
record_id=1 severity=error time="2026-Mar-03 07:05:21.952463" name="Global" msg="No such file or directory: Failed to open 'gege' for reading"
Installation failed. System not modified.
Could not fulfill request: No such file or directory: Failed to open 'gege' for reading
```
exit with code 1.


If mender-update commit is called after install, but before reboot, it should print something like
```
record_id=1 severity=info time="2026-Mar-03 07:38:33.551925" name="Global" msg="Update Module output (stderr): Mounted root does not match boot loader environment (/dev/mmcblk0p3)!"
record_id=2 severity=error time="2026-Mar-03 07:38:33.552810" name="Global" msg="Commit failed: Process returned non-zero exit status: ArtifactCommit: Process exited with status 1"
Installation failed. Rolled back modifications.
```
exit with code 1.

If mender-update commit is called after install and reboot, it should make the new rootfs active and print
```
Committed.
```
exit with code 0.

mender-update install should fail if there is already an installed but not committed update, and print
```
record_id=1 severity=error time="2026-Mar-03 07:09:26.999642" name="Global" msg="Operation now in progress: Update already in progress. Please commit or roll back first"
Installation failed. System not modified.
Could not fulfill request: Operation now in progress: Update already in progress. Please commit or roll back first
```
exit with code 1.


data/0000.tar.gz contains an ext4 image. Make the content available somehow. I need to inspect /etc/issue in the new rootfs after installation.

### mender-update for Core OS Customization Set updates

Core OS Customization Set artifacts use the Mender payload type
`os-customization`. They share the Mender update transaction with rootfs and
application updates: while any update is installed but not committed or rolled
back, another update cannot be installed.

For the mock, a customization payload contains only its `manifest.json`; the
mock does not simulate extraction of the `etc/` tree or the OverlayFS behavior
of the customization manager.

The manifest's optional `health_checks` decides the result after the reboot
requested by a successful installation. Test artifacts use only checks of this
form:

```json
{
  "type": "command",
  "command": ["true"]
}
```

or `command: ["false"]`. All health checks must pass. `true` succeeds and
`false` fails. The mock shall reject unsupported health-check types or command
forms.

`mender-update install <image-file>` stages the manifest as a pending
customization update and prints the regular successful-install message:

```
Installed, but not committed.
Use 'commit' to update, or 'rollback' to roll back the update.
```

It must not change `/etc`. On the following `reboot`, the mock evaluates the
health checks. A passing set becomes the active last-known-good customization;
`mender-update commit` then completes the Mender transaction. A failing set is
marked as failed and may be cleared with `mender-update rollback`. Subsequent
reboot calls must not turn a failed set into a successful deployment.

### os-customization-set command

The mock shall provide `os-customization-set status`. It shall print JSON with
the status fields used by the real command: `active_slot`, `active_version`,
`last_good_slot`, `last_good_version`, `candidate_slot`, `candidate_version`,
`candidate_state`, `candidate_attempts`, and `factory_version`.

The status is backed by the same state as `mender-update`. Before a
customization deployment, all version and slot fields are `null`. While an
artifact is staged, it reports a `pending` candidate. After reboot, a passing
health check makes the candidate the active last-known-good version; a failing
check reports `candidate_state: "rolled-back"` and retains the previous active
version. The mock does not model factory customizations, so `factory_version`
is always `null`.

### Reboot command

Shall simulate a system reboot. It should print "Rebooting..." and exit with code 0. After reboot, the pointer to the current rootfs should be switched to the new rootfs if it was installed but not committed.

If reboot is called with an installed but not committed update it should switch the pointer back to the old rootfs, simulating a failed update.

If environment variable `MOCK_MENDER_KILL_PARENT` is set to `yes`, reboot should kill its parent process to simulate that a system reboot terminated the current process tree.

### mender-update for application updates

On the real target, application updates are done with mender app module https://raw.githubusercontent.com/ci4rail/meta-ci4rail-bsp/refs/heads/scarthgap/recipes-mender/mender-docker-compose/files/app and app-sub-module "docker-compose" https://raw.githubusercontent.com/ci4rail/meta-ci4rail-bsp/refs/heads/scarthgap/recipes-mender/mender-docker-compose/files/docker-compose.

Application states are store under /data/mender-app/{application-name]{[-previous]}
When rollout is made, 
- old application directory is renamed to {application-name}-previous, 
- old application is stopped via docker-compose down
- new application is installed to {application-name},
- new application is started via docker-compose up -d


<file>.mender is again a tar file containing

version
manifest
header.tar.gz or header.tar
data/0000.tar

Inside header.tar.gz or header.tar:

header-info
headers/0000/type-info
headers/0000/meta-data

header-info looks like this:
```
{"payloads":[{"type":"app"}],"artifact_provides":{"artifact_name":"app-nginx-demo-moducop-cpu01-linux_arm64-8f249b9"},"artifact_depends":{"device_type":["moducop-cpu01"]}}
```
headers/0000/meta-data:
{"application_name":"nginx-demo","images":["1d13701a5f9f3fb01aaa88cef2344d65b6b5bf6b7d9fa4cf0dca557a8d7702ba","25109184c71bdad752c8312a8623239686a9a2071e8825f20acb8f2198c3f659"],"orchestrator":"docker-compose","platform":"linux/arm64","version":"1.0"}

data/0000.tar contains:
images.tar.gz
manifests.tar.gz

manifests.tar.gz contains:
manifests/
manifests/docker-compose.yaml
manifests/.env


The mock shall extract the content of manifests.tar to /data/mender-app/{application-name}/manifests. images.tar.gz can be ignored for the mock.

The mocked docker command should show running containers based on the docker-compose.yaml file in that directory. It shall reflect docker labels

$ docker ps --format '{{.Names}}\t{{.Labels}}'
```
nxginx-demo-web-1       maintainer=NGINX Docker Maintainers <docker-maint@nginx.com>,software-version=8f249b9,com.docker.compose.config-hash=2007b41087eb4fab7bc8aed42d2bb9f4d961c5e8e6fb8e2d00b008c84787c894,com.docker.compose.oneoff=False,com.docker.compose.project=nxginx-demo,com.docker.compose.project.working_dir=/data/mender-docker-compose/new/manifests,com.docker.compose.version=2.26.0,com.docker.compose.container-number=1,com.docker.compose.depends_on=,com.docker.compose.image=sha256:128568fed7ff6f758ccfd95b4d4491a53d765e5553c46f44889c6c5f136c8c5b,com.docker.compose.project.config_files=/data/mender-docker-compose/new/manifests/docker-compose.yaml,com.docker.compose.service=web
```

Important labels are only: com.docker.compose.project, and the labels added by the docker-compose file, e.g. software-version. Other labels can be ignored for the mock.

#### Exact behavior of mender-update for application updates

Successful output shall take at least 5 seconds and print progress every second. After successful installation, it should print
```
Update Module doesn't support rollback. Committing immediately.
Installed and committed.
```
(no commit or reboot needed for application updates)

If same version of the application is already installed, it should perform a normal installation.

If an application OR rootfs update is already installed but not committed, it should print
```
Installation failed. System not modified.
Could not fulfill request: Operation now in progress: Update already in progress. Please commit or roll back first
```

#### Error injection

It shall be possible to inject errors into the mender-update command via an err-inject command.

* after stopping old containers, mender-update shall exit
* after renaming old application directory, mender-update shall exit
* after extracting new application, but before starting new containers, mender-update shall exit
* docker-compose-up-failed shall simulate a persistent `docker compose up` failure caused by a nonexistent bind-mount source

If environment variable `MOCK_MENDER_KILL_PARENT` is set to `yes`, hitting an injected error shall additionally kill the parent process.

After error injection in application update, the system should reject new updates with "Update already in progress". It shall be possible to commit the pending update, but this commit should leave the system in an inconsistent state.  

The next application install after the commit should output: "Installation failed, and Update Module does not support rollback. System may be in an inconsistent state.". In the real system, the only way out of this situation is to remove all manifest folders data/mender-app/nginx-demo[-previous]. After that, the system should be back to normal and allow new updates.



### docker command

The mocked docker command should support:

- docker ps --format '{{.Names}}\t{{.Labels}}'
- docker compose -p {project] down

### preparefs command

The preparefs command should prepare the filesystem for the mock. It should create the necessary directories and files, and set up the initial state of the system. It should be idempotent, so it can be run multiple times without causing issues. It should:

- Create /etc/issue with the following content:
```
TDX Wayland with XWayland 7.1.0-devel-20260210154127+build.0 (scarthgap) \n \l
Moducop-CPU01_Standard-Image_v2.7.0.40ee657.20260218.1208
```

- Create /proc/sys/kernel/random/boot_id with a random UUID. 

### boot_id simulation

All commands that simulate a reboot should update the /proc/sys/kernel/random/boot_id file with a new random UUID to simulate a new boot. (reboot and mender-update after hitting the error injection point that simulates a reboot)


### io4edge-cli command

In reality, io4edge-cli manages "io4edge" devices. I.e. it can read hw and fw version, and can update the firmware. For the mock, we only need to simulate the following commands:

Each device has a device id which is used to address the device in the network.
Simulate 5 devices, named
- S100-IUO16-USB-EXT1-UC1
- S100-IUO16-USB-EXT1-UC2
- S100-IUO16-USB-EXT1-UC3
- S100-IUO16-USB-EXT1-UC4
- S100-IUO16-USB-EXT1-UC5

For the mock, we only need to simulate the following commands:

$ io4edge-cli -d S100-IUO16-USB-EXT1-UC1 fw
Firmware name: fw_iou016_default, Version 1.0.0

-> report here last updated firmware version for each device. Initially, report 1.0.0

$ io4edge-cli -d S100-IUO16-USB-EXT1-UC1 hw
Hardware name: S100-IUO16-00-00001, rev: 0, serial: b4e31793-f660-4e2e-af20-c175186b95be

-> just return a different serial for each device, and the same hardware name and revision.

$ io4edge-cli -d S100-IUO16-USB-EXT1-UC1  load-firmware fw_iou016_default-1.2.0.fwpkg

The fwpkg file is a tar file containing
./manifest.json
fw-iou16-00-default.bin

manifest.json contains:
{
  "name": "iou16-00-default",
  "version": "1.0.0-rc.4",
  "file": "fw-iou16-00-default.bin",
  "compatibility": {
    "hw": "s100-iou16",
    "major_revs": [
      1
    ]
  }
}

load shall take around 10 seconds to complete, and print progress every second. After successful load, it should print the just loaded firmware

```
Reconnecting to restarted device.
Firmware name: fw_iou016_default, Version 1.0.2
```

io4edge-cli scan shall list all known devices.
$ io4edge-cli scan
DEVICE ID               IP              HARDWARE        SERIAL
S100-IUO16-USB-EXT1-UC1 192.168.200.1   s100-iou16      b4e31793-f660-4e2e-af20-c175186b95be
S100-IUO16-USB-EXT1-UC2 192.168.201.1   s100-iou16      <serial>
...

### ee-inv

ee-inv returns the hardware information of moducop.

For the mock, just return the following fixed output

$ ee-inv /sys/bus/i2c/devices/3-0050/eeprom@256:256
{
  "vendor": "Ci4Rail",
  "model": "S100-MLC01",
  "variant": 0,
  "majorVersion": 1,
  "serial": "12345"
}
