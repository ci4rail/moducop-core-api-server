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