DROP DATABASE IF EXISTS isuumo;

CREATE DATABASE isuumo;

DROP TABLE IF EXISTS isuumo.estate;

DROP TABLE IF EXISTS isuumo.chair;

CREATE TABLE isuumo.estate (
    id SMALLINT UNSIGNED NOT NULL PRIMARY KEY,
    name VARCHAR(32) NOT NULL,
    description VARCHAR(128) NOT NULL,
    thumbnail VARCHAR(128) NOT NULL,
    address VARCHAR(128) NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    rent MEDIUMINT NOT NULL,
    door_height TINYINT UNSIGNED NOT NULL,
    door_width TINYINT UNSIGNED NOT NULL,
    features VARCHAR(64) NOT NULL,
    popularity INTEGER NOT NULL,
    popularity_desc MEDIUMINT AS (-popularity) INVISIBLE,
    INDEX (`rent`, `door_width`),
    INDEX (`rent`, `door_height`),
    INDEX (`popularity_desc`)
);


CREATE TABLE isuumo.chair (
    id SMALLINT UNSIGNED NOT NULL PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    description VARCHAR(128) NOT NULL,
    thumbnail VARCHAR(128) NOT NULL,
    price SMALLINT UNSIGNED NOT NULL,
    height TINYINT UNSIGNED NOT NULL,
    width TINYINT UNSIGNED NOT NULL,
    depth TINYINT UNSIGNED NOT NULL,
    color VARCHAR(64) NOT NULL,
    features VARCHAR(64) NOT NULL,
    kind VARCHAR(64) NOT NULL,
    popularity MEDIUMINT NOT NULL,
    popularity_desc MEDIUMINT AS (-popularity) INVISIBLE,
    stock TINYINT UNSIGNED NOT NULL,
    INDEX (`price`, `stock`),
    INDEX (`height`, `stock`),
    INDEX (`kind`, `stock`),
    INDEX (`popularity_desc`)
);
