# Mocks overview

We need mocks for all interfaces used by the moducop-core-api-server. This includes:
- Filesystem operations 
- mender-update command
- reboot command
- docker command

All mock commands should be implemented as Go CLI commands.

## Filesystem operations

The moducop-core-api-server will run in a chroot environment, the mock shall simulate the following folders:
- /etc/issue - mirror the /etc/issue file of the current rootfs
- /data/mender-app/{application-name]/manifest - contains the manifest folder of mender artifacts for application updates. Contains docker-compose.yaml

## Mocks interaction

The mock commands and the provided filesystem shall interact with each other to simulate the behavior of the actual system. For example, when mender-update install is called, it should write the new rootfs to a temporary directory and update the /etc/issue file to reflect the new rootfs. When reboot is called, it should switch the pointer to the current rootfs to the new rootfs if it was installed but not committed, or switch back to the old rootfs if reboot is called with an installed but not committed update.

Also the "mender-update install" command, when called with an application update artifact, should write the docker-compose.yaml to /data/mender-app/{application-name]/manifest/docker-compose.yaml, and the mocked docker command should show running containers based on the docker-compose.yaml file in that directory.

## mender-update command

mender-update is used for
- rootfs updates
- application updates

They are NOT independent, they share internal state. While one update is installed but not committed, no other update can be installed. If a rootfs update is installed but not committed, application updates cannot be installed, and vice versa.

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

### Reboot command

Shall simulate a system reboot. It should print "Rebooting..." and exit with code 0. After reboot, the pointer to the current rootfs should be switched to the new rootfs if it was installed but not committed.

If reboot is called with an installed but not committed update it should switch the pointer back to the old rootfs, simulating a failed update.

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
{"payloads":[{"type":"docker-compose"}],"artifact_provides":{"artifact_name":"nginx-demo-8f249b9"},"artifact_depends":{"device_type":["moducop-cpu01"]}}
```
headers/0000/meta-data:
{"project_name":"nxginx-demo","version":"1"}

data/0000.tar contains:
images.tar.gz
manifests.tar

manifests.tar contains:
manifests/
manifests/docker-compose.yaml
manifests/.env


The mock shall extract the content of manifests.tar to /data/mender-app/{application-name}/manifest. images.tar.gz can be ignored for the mock.

The mocked docker command should show running containers based on the docker-compose.yaml file in that directory. It shall reflect docker labels

$ docker ps --format '{{.Names}}\t{{.Labels}}'
```
nxginx-demo-web-1       maintainer=NGINX Docker Maintainers <docker-maint@nginx.com>,software-version=8f249b9,com.docker.compose.config-hash=2007b41087eb4fab7bc8aed42d2bb9f4d961c5e8e6fb8e2d00b008c84787c894,com.docker.compose.oneoff=False,com.docker.compose.project=nxginx-demo,com.docker.compose.project.working_dir=/data/mender-docker-compose/new/manifests,com.docker.compose.version=2.26.0,com.docker.compose.container-number=1,com.docker.compose.depends_on=,com.docker.compose.image=sha256:128568fed7ff6f758ccfd95b4d4491a53d765e5553c46f44889c6c5f136c8c5b,com.docker.compose.project.config_files=/data/mender-docker-compose/new/manifests/docker-compose.yaml,com.docker.compose.service=web
```

Important labels are only: com.docker.compose.project, and the labels added by the docker-compose file, e.g. software-version. Other labels can be ignored for the mock.


### docker command

The mocked docker command should support:

- docker ps --format '{{.Names}}\t{{.Labels}}'
- docker compose -p {project] down