/*for sqlite*/
create table tasks
(
    id          integer primary key autoincrement,
    config      text        not null,
    start_time  timestamp default {{ if eq .engine "sqlite3" }} current_timestamp {{ else if eq .engine "mysql" }} now() {{end}},
    finish_time timestamp default null,
    status      varchar(16) not null,
    result      text      default null,
    message     varchar(4096)
);
