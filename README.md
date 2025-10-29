# codec-svr

Servicio TCP (Codec8 Extended) para recepción y procesamiento de datos GPS Teltonika.

## Características
- Escucha por TCP en el puerto 8001.
- Decodifica Codec8 Extended.
- Envía datos a otro servicio vía gRPC.
- Mantiene conexión bidireccional con los dispositivos.
- Métricas en `:9000/metrics` y healthcheck en `:9000/healthz`.

## Estructura
Basada en Clean Architecture para alta mantenibilidad.

## Comandos útiles
```bash
go run ./cmd/server
go build -o codec-svr ./cmd/server
sudo systemctl enable codec-svr
sudo systemctl status codec-svr

## Pendientes
## 1. revisar por que redis no está actualizando
## 2. el orden de los estados de IO
## 3. aplicar la emision de datos por gRPC
 