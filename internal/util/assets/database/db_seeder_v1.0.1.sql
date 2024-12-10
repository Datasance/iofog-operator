START TRANSACTION;

INSERT INTO "Registries" (url, is_public, secure, certificate, requires_cert, user_name, password, user_email)
VALUES 
    ('registry.hub.docker.com', true, true, '', false, '', '', ''),
    ('from_cache', true, true, '', false, '', '', '');
   

INSERT INTO "CatalogItems" (name, description, category, publisher, disk_required, ram_required, picture, config_example, is_public, registry_id)
VALUES 
    ('Networking Tool', 'The built-in networking tool for Eclipse ioFog.', 'SYSTEM', 'Eclipse ioFog', 0, 0, 'none.png', NULL, false, 1),
    ('RESTBlue', 'REST API for Bluetooth Low Energy layer.', 'SYSTEM', 'Eclipse ioFog', 0, 0, 'none.png', NULL, false, 1),
    ('HAL', 'REST API for Hardware Abstraction layer.', 'SYSTEM', 'Eclipse ioFog', 0, 0, 'none.png', NULL, false, 1),
    ('Diagnostics', '0', 'UTILITIES', 'Eclipse ioFog', 0, 0, 'images/build/580.png', NULL, true, 1),
    ('Hello Web Demo', 'A simple web server to test Eclipse ioFog.', 'UTILITIES', 'Eclipse ioFog', 0, 0, 'images/build/4.png', NULL, true, 1),
    ('Open Weather Map Data', 'A stream of data from the Open Weather Map API in JSON format', 'SENSORS', 'Eclipse ioFog', 0, 0, 'images/build/8.png', NULL, true, 1),
    ('JSON REST API', 'A configurable REST API that gives JSON output', 'UTILITIES', 'Eclipse ioFog', 0, 0, 'images/build/49.png', NULL, true, 1),
    ('Temperature Converter', 'A simple temperature format converter', 'UTILITIES', 'Eclipse ioFog', 0, 0, 'images/build/58.png', NULL, true, 1),
    ('JSON Sub-Select', 'Performs sub-selection and transform operations on any JSON messages', 'UTILITIES', 'Eclipse ioFog', 0, 0, 'images/build/59.png', NULL, true, 1),
    ('Humidity Sensor Simulator', 'Humidity Sensor Simulator for Eclipse ioFog', 'SIMULATOR', 'Eclipse ioFog', 0, 0, 'images/build/simulator.png', NULL, true, 1),
    ('Seismic Sensor Simulator', 'Seismic Sensor Simulator for Eclipse ioFog', 'SIMULATOR', 'Eclipse ioFog', 0, 0, 'images/build/simulator.png', NULL, true, 1),
    ('Temperature Sensor Simulator', 'Temperature Sensor Simulator for Eclipse ioFog', 'SIMULATOR', 'Eclipse ioFog', 0, 0, 'images/build/simulator.png', NULL, true, 1),
    ('Common Logging', 'Container which gathers logs and provides REST API for adding and querying logs from containers', 'UTILITIES', 'Eclipse ioFog', 0, 0, 'none.png', '{"access_tokens": ["Some_Access_Token"], "cleanfrequency": "1h40m", "ttl": "24h"}', false, 1),
    ('JSON Generator', 'Container generates ioMessages with contentdata as complex JSON object.', 'UTILITIES', 'Eclipse ioFog', 0, 0, 'none.png', '{}', true, 1),
    ('Router', 'The built-in router for Datasance PoT.', 'SYSTEM', 'Eclipse ioFog', 0, 0, 'none.png', NULL, false, 1),
    ('Proxy', 'The built-in proxy for Datasance PoT.', 'SYSTEM', 'Eclipse ioFog', 0, 0, 'none.png', NULL, false, 1);


   
INSERT INTO "FogTypes" (id, name, image, description, network_catalog_item_id, hal_catalog_item_id, bluetooth_catalog_item_id)
VALUES 
    (0, 'Unspecified', 'iointegrator0.png', 'Unspecified device. Fog Type will be selected on provision', 1, 3, 2),
    (1, 'Standard Linux (x86)', 'iointegrator1.png', 'A standard Linux server of at least moderate processing power and capacity. Compatible with common Linux types such as Ubuntu, Red Hat, and CentOS.', 1, 3, 2),
    (2, 'ARM Linux', 'iointegrator2.png', 'A version of ioFog meant to run on Linux systems with ARM processors. Microservices for this ioFog type will be tailored to ARM systems.', 1, 3, 2);

UPDATE "Fogs"
SET fog_type_id = 0
WHERE fog_type_id IS NULL;
   

INSERT INTO "CatalogItemImages" (catalog_item_id, fog_type_id, container_image)
VALUES 
    (1, 1, 'iofog/core-networking'),
    (1, 2, 'iofog/core-networking-arm'),
    (2, 1, 'iofog/restblue'),
    (2, 2, 'iofog/restblue-arm'),
    (3, 1, 'ghcr.io/datasance/hal'),
    (3, 2, 'ghcr.io/datasance/hal-arm'),
    (4, 1, 'iofog/diagnostics'),
    (4, 2, 'iofog/diagnostics-arm'),
    (5, 1, 'iofog/hello-web'),
    (5, 2, 'iofog/hello-web-arm'),
    (6, 1, 'iofog/open-weather-map'),
    (6, 2, 'iofog/open-weather-map-arm'),
    (7, 1, 'iofog/json-rest-api'),
    (7, 2, 'iofog/json-rest-api-arm'),
    (8, 1, 'iofog/temperature-conversion'),
    (8, 2, 'iofog/temperature-conversion-arm'),
    (9, 1, 'iofog/json-subselect'),
    (9, 2, 'iofog/json-subselect-arm'),
    (10, 1, 'iofog/humidity-sensor-simulator'),
    (10, 2, 'iofog/humidity-sensor-simulator-arm'),
    (11, 1, 'iofog/seismic-sensor-simulator'),
    (11, 2, 'iofog/seismic-sensor-simulator-arm'),
    (12, 1, 'iofog/temperature-sensor-simulator'),
    (12, 2, 'iofog/temperature-sensor-simulator-arm'),
    (13, 1, 'iofog/common-logging'),
    (13, 2, 'iofog/common-logging-arm'),
    (14, 1, 'iofog/json-generatorg'),
    (14, 2, 'iofog/json-generator-arm'),
    (15, 1, 'ghcr.io/datasance/router:latest'),
    (15, 2, 'ghcr.io/datasance/router:latest'),
    (16, 1, 'ghcr.io/datasance/proxy:latest'),
    (16, 2, 'ghcr.io/datasance/proxy:latest');

   
UPDATE "CatalogItems" 
SET config_example = '{"citycode":"5391997","apikey":"6141811a6136148a00133488eadff0fb","frequency":1000}' 
WHERE name = 'Open Weather Map Data';

UPDATE "CatalogItems" 
SET config_example = '{"buffersize":3,"contentdataencoding":"utf8","contextdataencoding":"utf8","outputfields":{"publisher":"source","contentdata":"temperature","timestamp":"time"}}' 
WHERE name = 'JSON REST API';

UPDATE "CatalogItems" 
SET config_example = '{}' 
WHERE name = 'JSON Sub-Select';

UPDATE "CatalogItems" 
SET is_public = true 
WHERE name = 'Common Logging';
    
COMMIT;