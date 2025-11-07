# codec-svr

Servicio TCP (Codec8 Extended) para recepción y procesamiento de datos GPS Teltonika.

## Características
- Escucha por TCP en el puerto 8001.
- Decodifica Codec8 Extended.
- Envía datos a otro servicio vía gRPC.
- Mantiene conexión bidireccional con los dispositivos.
- Métricas en `:9000/metrics` y healthcheck en `:9000/healthz`.

## Estructura
Basada en "Clean Architecture/Hexagonal" con principios de Domain-Driven Design (DDD) simplificados para Go.

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
## 4. quita la expiracion de redis
## 5. guarda la clave "inputs" y valor de izquierda a derecha "00001"
 
## Documentation
## https://wiki.teltonika-gps.com/view/Teltonika_AVL_Protocols
## https://wiki.teltonika-gps.com/view/Teltonika_Data_Sending_Protocols
## https://wiki.teltonika-gps.com/view/FMC125_Teltonika_Data_Sending_Parameters_ID