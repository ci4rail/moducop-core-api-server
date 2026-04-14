# moducop-core-api-server

moducop-core-api-server is a REST API Server that abstracts management functions of moducop devices (embedded Linux devices using ARM64 processor based on Toradex Verdin SoM).

Functionality:
- Linux Rootfs Update via mender https://docs.mender.io/
- Application Update via mender-artifact and docker compose
- Secondary device updates for io4edge devices: https://docs.ci4rail.com/user-docs/io4edge/ 
- Reboot
- May be extended later to support log file retrieval, metrics retrieval, etc.

## API Spec

OpenAPI spec is here: https://github.com/ci4rail/moducop-core-api-server-spec

## Integration Testing

To run the tests, 
* ensure you have python3 and the packages listed in `requirements.txt` installed

### Testing with Mocks
For testing the moducop-core-api-server on developer machines and in CI pipelines, there is a mock implementation of the commands used by the server. See mocks/AGENTS.md for details.

* run `make mocks`
* run `make test-with-mocks`.

### Testing with real devices

Tests can also run on real devices, currently Moducops with Verdin IMX8MM only. 
Have the moducop connected to the same network as the machine running the tests, enter it's IP address in `tests/Makefile` and run `cd tests && make real-test`.
Note: This reset the device to a factory-like state, so only run this if you are sure that this is ok.

## Installation 

As long as the core-api-server is not part of the rootfs image, it can be installed by deploying the provided tarball from the github releases to the device. 

copy the tarball to the device, for example to /data, and run the following command on the device:
```
tar -C /data -xf  /data/core-api-server-<version>-linux-arm64-deployment.tar.gz
reboot
```