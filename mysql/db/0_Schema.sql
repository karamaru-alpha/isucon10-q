DROP DATABASE IF EXISTS isuumo;

CREATE DATABASE isuumo;

DROP TABLE IF EXISTS isuumo.estate;

DROP TABLE IF EXISTS isuumo.chair;

CREATE TABLE isuumo.estate (
    id INTEGER NOT NULL PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    description VARCHAR(4096) NOT NULL,
    thumbnail VARCHAR(128) NOT NULL,
    address VARCHAR(128) NOT NULL,
    latitude DOUBLE PRECISION NOT NULL,
    longitude DOUBLE PRECISION NOT NULL,
    rent INTEGER NOT NULL,
    door_height INTEGER NOT NULL,
    door_width INTEGER NOT NULL,
    features VARCHAR(64) NOT NULL,
    popularity INTEGER NOT NULL,
    popularity_desc INTEGER AS (-popularity) INVISIBLE,
    INDEX (`rent`, `id`),
    INDEX (`rent`, `door_width`),
    INDEX (`popularity_desc`, `id`)
);


CREATE TABLE isuumo.chair (
    id INTEGER NOT NULL PRIMARY KEY,
    name VARCHAR(64) NOT NULL,
    description VARCHAR(4096) NOT NULL,
    thumbnail VARCHAR(128) NOT NULL,
    price INTEGER NOT NULL,
    height INTEGER NOT NULL,
    width INTEGER NOT NULL,
    depth INTEGER NOT NULL,
    color VARCHAR(64) NOT NULL,
    features VARCHAR(64) NOT NULL,
    kind VARCHAR(64) NOT NULL,
    popularity INTEGER NOT NULL,
    popularity_desc INTEGER AS (-popularity) INVISIBLE,
    stock INTEGER NOT NULL,
    INDEX (`price`, `id`),
    INDEX (`price`, `stock`),
    INDEX (`popularity_desc`, `id`)
);
