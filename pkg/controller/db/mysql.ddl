create table if not exists tasks
(
    id          integer primary key autoincrement,
    config      text        not null,
    start_time  timestamp default now(),
    finish_time timestamp default null,
    status      varchar(16) not null,
    result      text      default null,
    message     varchar(4096)
    );
