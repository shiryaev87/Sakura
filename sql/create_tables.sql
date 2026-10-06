create table plots(
id serial primary key,
number text not null,
area numeric not null,
price numeric  not null,
status text not null default 'available',
created_at timestamp default now()
);

insert into plots (number , area, price ,status) values 
('1', 6.6 , 500000, 'available'),
('2', 4.5 , 500000, 'available'),
('3', 5 , 500000, 'available'),
('4', 6 , 500000, 'available'),
('5', 8 , 500000, 'available'),
('6', 7 , 500000, 'available');


insert into plots (number , area, price ,status) values 
('11', 6.6 , 500000, 'available');

alter table plots add column svg_points text;

UPDATE plots SET svg_points = '10,10 110,10 110,80 10,80'        WHERE number = 1;
UPDATE plots SET svg_points = '120,10 220,10 220,80 120,80'      WHERE number = 2;
UPDATE plots SET svg_points = '230,10 330,10 330,80 230,80'      WHERE number = 3;
UPDATE plots SET svg_points = '340,10 440,10 440,80 340,80'      WHERE number = 4;
UPDATE plots SET svg_points = '10,90 110,90 110,160 10,160'      WHERE number = 5;
UPDATE plots SET svg_points = '120,90 220,90 220,160 120,160'    WHERE number = 6;
UPDATE plots SET svg_points = '230,90 330,90 330,160 230,160'    WHERE number = 7;
UPDATE plots SET svg_points = '340,90 440,90 440,160 340,160'    WHERE number = 8;


update plots set status = 'available' ;

UPDATE plots 
SET svg_points = '0,0 50,0 50,50 0,50' 
WHERE svg_points IS NULL;



--создаем таблицу с пользователями

create table users (
	id SERIAL primary key,
	email text unique not null,
	password_hash text not null,
	created_at timestamp default now()
);

alter table plots add column reserved_by integer references users(id);

select * from users


alter table users add column role  text not null default 'client';

update users  set  role  = 'admin' where  email = 'shiryaev_87@mail.ru'