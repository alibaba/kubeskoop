create table if not exists `tasks`
(
    `id`          integer AUTO_INCREMENT,
    `config`      text        not null,
    `start_time`  timestamp default now(),
    `finish_time` timestamp null default null,
    `status`      varchar(16) not null,
    `result`      text      default null,
    `message`     varchar(4096),
    PRIMARY KEY(`id`)
);
