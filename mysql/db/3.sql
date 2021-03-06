ALTER TABLE isuumo.estate ADD COLUMN geom POINT;

UPDATE isuumo.estate SET geom=ST_PointFromText(CONCAT('POINT(', latitude, ' ', longitude, ')'));

ALTER TABLE isuumo.estate MODIFY COLUMN geom POINT NOT NULL DEFAULT '' INVISIBLE, ADD SPATIAL INDEX(geom);
