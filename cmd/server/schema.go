package main

const schemaSQL = `
create table if not exists topics (
	id bigserial primary key,
	title text not null,
	chapter text not null default '',
	description text not null default '',
	difficulty int not null default 1,
	created_at timestamptz not null default now()
);

create table if not exists theory_items (
	id bigserial primary key,
	topic_id bigint not null references topics(id) on delete cascade,
	type text not null check (type in ('definition', 'theorem', 'criterion', 'property', 'example', 'proof')),
	title text not null,
	statement text not null default '',
	explanation text not null default '',
	proof_outline text not null default '',
	proof_full text not null default '',
	source_page int,
	created_at timestamptz not null default now()
);

create table if not exists cards (
	id bigserial primary key,
	topic_id bigint not null references topics(id) on delete cascade,
	theory_item_id bigint references theory_items(id) on delete set null,
	type text not null check (type in ('definition', 'theorem', 'criterion', 'formula', 'proof_outline')),
	front text not null,
	back text not null,
	hint text not null default '',
	difficulty int not null default 1,
	created_at timestamptz not null default now()
);

create table if not exists questions (
	id bigserial primary key,
	topic_id bigint not null references topics(id) on delete cascade,
	theory_item_id bigint references theory_items(id) on delete set null,
	type text not null check (type in ('prove', 'derive', 'explain', 'example', 'true_false', 'formulate', 'oral')),
	text text not null,
	expected_answer text not null default '',
	short_answer text not null default '',
	difficulty int not null default 1,
	tags text[] not null default '{}',
	created_at timestamptz not null default now()
);

create table if not exists proof_choice_games (
	id bigserial primary key,
	theory_item_id bigint not null references theory_items(id) on delete cascade,
	question text not null,
	correct_outline text not null,
	wrong_outline_1 text not null,
	wrong_outline_2 text not null,
	wrong_outline_3 text not null,
	explanation text not null default '',
	created_at timestamptz not null default now()
);

create table if not exists attempts (
	id bigserial primary key,
	demo_student_id bigint not null default 1,
	target_type text not null check (target_type in ('card', 'question', 'proof_game')),
	target_id bigint not null,
	mode text not null check (mode in ('cards', 'formulations', 'proof_game', 'oral', 'variant')),
	grade text not null check (grade in ('bad', 'almost', 'good', 'perfect')),
	notes text not null default '',
	created_at timestamptz not null default now()
);

create index if not exists idx_theory_items_topic_id on theory_items(topic_id);
create index if not exists idx_cards_topic_id on cards(topic_id);
create index if not exists idx_questions_topic_id on questions(topic_id);
create index if not exists idx_attempts_demo_student on attempts(demo_student_id, created_at desc);
`

const seedSQL = `
insert into topics (id, title, chapter, description, difficulty) values
	(1, 'Числовые ряды и признаки сходимости', 'Ряды', 'Признаки Куммера, Раабе, Гаусса, Лейбница и монотонность.', 3),
	(2, 'Функции двух переменных', 'Ф2П', 'Компакты, непрерывность, равномерная непрерывность и теорема Кантора.', 3),
	(3, 'Векторные функции и обратное отображение', 'Дифференциальное исчисление', 'Обратная векторная функция и структура доказательства.', 4),
	(4, 'Условный экстремум', 'Экстремумы', 'Необходимые условия локального условного экстремума.', 4),
	(5, 'Линейные и функциональные системы', 'Функциональный анализ', 'Линейная зависимость и функциональная независимость.', 4);

insert into theory_items (id, topic_id, type, title, statement, explanation, proof_outline, proof_full) values
	(1, 2, 'theorem', 'Теорема Кантора для функции двух переменных',
	 'Если функция двух переменных непрерывна на компакте, то она равномерно непрерывна на этом компакте.',
	 'Смысл: на замкнутом и ограниченном множестве непрерывность можно контролировать одним общим delta для всех точек.',
	 'Доказываем от противного: берём две последовательности точек компакта, расстояние между ними стремится к нулю, а значения функции отличаются хотя бы на epsilon. По компактности выделяем сходящуюся подпоследовательность, затем используем непрерывность и получаем противоречие.',
	 'Полное доказательство будет внесено из учебника после разбора PDF.'),
	(2, 1, 'criterion', 'Признак Раабе',
	 'Для положительного ряда исследуется предел n(a_n/a_{n+1} - 1). Если предел больше 1, ряд сходится; если меньше 1, ряд расходится.',
	 'Признак уточняет сравнение соседних членов и часто сильнее признака Даламбера.',
	 'Выводим из признака Куммера, выбирая подходящую последовательность b_n = n и раскрывая выражение Куммера.',
	 'Полное доказательство будет внесено из учебника после разбора PDF.'),
	(3, 1, 'criterion', 'Признак Гаусса',
	 'Признак сходимости положительного ряда через асимптотическое разложение отношения a_n/a_{n+1}.',
	 'Используется, когда отношение соседних членов имеет разложение с членом порядка 1/n.',
	 'Сводим асимптотику отношения соседних членов к сравнению с обобщенным гармоническим рядом.',
	 'Полное доказательство будет внесено из учебника после разбора PDF.'),
	(4, 4, 'theorem', 'Необходимое условие локального условного экстремума',
	 'В точке условного экстремума при регулярной связи градиент целевой функции выражается линейной комбинацией градиентов связей.',
	 'Это основа метода множителей Лагранжа.',
	 'Переходим к допустимым приращениям вдоль поверхности связи и требуем, чтобы первая вариация на касательном пространстве обращалась в ноль.',
	 'Полное доказательство будет внесено из учебника после разбора PDF.'),
	(5, 3, 'theorem', 'Теорема об обратной векторной функции',
	 'При непрерывной дифференцируемости и невырожденном якобиане отображение локально обратимо, а обратное отображение дифференцируемо.',
	 'Показывает, когда систему уравнений можно локально разрешить относительно исходных переменных.',
	 'Идея: линеаризация имеет обратимый главный член; затем через оценку остатка и принцип сжатия строится локальная обратная функция.',
	 'Полное доказательство будет внесено из учебника после разбора PDF.'),
	(6, 5, 'example', 'Линейно зависимая, но функционально независимая система',
	 'Нужно проверить, может ли конечная система линейных функций быть линейно зависимой, но функционально независимой.',
	 'Это вопрос на различение алгебраической зависимости функций как векторов и функциональной зависимости значений.',
	 'Разобрать определения и показать совместимость или противоречие условий.',
	 'Эталонный разбор будет внесен после сверки с учебником.');

insert into cards (topic_id, theory_item_id, type, front, back, hint, difficulty) values
	(2, 1, 'theorem', 'Сформулировать теорему Кантора для функции двух переменных.', 'Если функция двух переменных непрерывна на компакте, то она равномерно непрерывна на этом компакте.', 'Компакт + непрерывность => равномерная непрерывность.', 2),
	(1, 2, 'criterion', 'Сформулировать признак Раабе.', 'Для положительного ряда рассматривают предел n(a_n/a_{n+1} - 1). Если он больше 1, ряд сходится; если меньше 1, расходится.', 'Положительный ряд, отношение соседних членов, граница 1.', 3),
	(1, 3, 'criterion', 'Сформулировать признак Гаусса.', 'Признак использует асимптотическое разложение отношения соседних членов и сводит вопрос к сравнению с p-рядом.', 'Нужно вспомнить разложение отношения a_n/a_{n+1}.', 4),
	(4, 4, 'theorem', 'Необходимое условие локального условного экстремума.', 'В регулярной точке условного экстремума градиент целевой функции является линейной комбинацией градиентов функций связи.', 'Множители Лагранжа.', 4),
	(3, 5, 'theorem', 'Сформулировать теорему об обратной векторной функции.', 'Если отображение C1 и его якобиан в точке невырожден, то в окрестности существует дифференцируемое обратное отображение.', 'Невырожденный якобиан.', 4);

insert into proof_choice_games (theory_item_id, question, correct_outline, wrong_outline_1, wrong_outline_2, wrong_outline_3, explanation) values
	(1, 'Как строится доказательство теоремы Кантора для функции двух переменных?',
	 'От противного: берём две близкие последовательности точек компакта с заметно разными значениями функции, выделяем сходящуюся подпоследовательность и через непрерывность получаем противоречие.',
	 'Строим интегральные суммы по разбиению компакта и доказываем их сходимость.',
	 'Применяем признак Даламбера к последовательности значений функции.',
	 'Ищем стационарные точки методом множителей Лагранжа.',
	 'Ключевые слова: от противного, компактность, подпоследовательность, непрерывность.'),
	(2, 'Как вывести признак Раабе из признака Куммера?',
	 'В признаке Куммера выбирают b_n = n, подставляют в выражение для соседних членов и получают предел n(a_n/a_{n+1} - 1).',
	 'Сначала доказывают равномерную непрерывность суммы ряда на компакте.',
	 'Берут производную общего члена и проверяют условие локального экстремума.',
	 'Заменяют ряд интегралом и применяют формулу Ньютона-Лейбница.',
	 'Здесь проверяется именно выбор вспомогательной последовательности в признаке Куммера.'),
	(5, 'Какова идея доказательства теоремы об обратной векторной функции?',
	 'Используем обратимость линейной части, оцениваем малый остаток и строим локальную обратную функцию через сжимающее отображение.',
	 'Доказываем, что множество внутренних точек компакта всегда замкнуто.',
	 'Применяем только теорему Вейерштрасса о максимуме и минимуме.',
	 'Разбиваем область на прямоугольники и сравниваем верхние и нижние суммы.',
	 'В доказательстве важна невырожденность якобиана и контроль нелинейного остатка.');

insert into questions (topic_id, theory_item_id, type, text, expected_answer, short_answer, difficulty, tags) values
	(2, 1, 'prove', 'Доказать теорему Кантора для ф2п.', 'Нужно сформулировать равномерную непрерывность, затем доказать от противного через компактность и непрерывность.', 'От противного + компактность + подпоследовательность + непрерывность.', 3, '{oral,variant,proof}'),
	(1, 2, 'derive', 'Вывести признак Раабе из признака Куммера.', 'Выбрать в признаке Куммера b_n = n и преобразовать условие к пределу n(a_n/a_{n+1} - 1).', 'Подстановка b_n = n в Куммер.', 4, '{oral,variant,proof}'),
	(1, 3, 'derive', 'Вывести признак Гаусса из признака Бертрана.', 'Нужно показать, как асимптотическое условие Гаусса получается как частный случай более общего признака Бертрана.', 'Сведение асимптотики отношения к Бертрану.', 4, '{oral,variant}'),
	(4, 4, 'prove', 'Доказать необходимое условие локального условного экстремума.', 'Через допустимые направления и обращение первой вариации в ноль получить линейную комбинацию градиентов связей.', 'Касательное пространство + множители Лагранжа.', 4, '{oral,variant,proof}'),
	(3, 5, 'prove', 'Доказать теорему об обратной векторной функции.', 'Показать локальную обратимость при невырожденном якобиане через оценку остатка и сжимающее отображение.', 'Невырожденный якобиан + сжатие.', 5, '{oral,variant,proof}'),
	(5, 6, 'explain', 'Существует ли конечная система линейных функций, которая является линейно зависимой, но функционально независимой?', 'Нужно аккуратно сравнить определения линейной зависимости функций и функциональной независимости и дать пример или доказать невозможность.', 'Вопрос на определения зависимости.', 4, '{oral,variant}'),
	(2, 1, 'true_false', 'Верно ли, что множество внутренних точек компакта является областью?', 'Не всегда. Множество внутренних точек может быть несвязным или пустым, а область должна быть открытой и связной.', 'Не всегда: может нарушаться связность.', 3, '{oral,variant}'),
	(1, 2, 'example', 'Привести пример неограниченной интегрируемой ф2п.', 'Нужен пример несобственной или ограниченно интегрируемой функции с особенностью на множестве меры ноль; точную формулировку сверим с учебником.', 'Пример с особенностью в точке.', 4, '{oral,variant}');

select setval(pg_get_serial_sequence('topics', 'id'), (select max(id) from topics));
select setval(pg_get_serial_sequence('theory_items', 'id'), (select max(id) from theory_items));
select setval(pg_get_serial_sequence('cards', 'id'), (select max(id) from cards));
select setval(pg_get_serial_sequence('proof_choice_games', 'id'), (select max(id) from proof_choice_games));
select setval(pg_get_serial_sequence('questions', 'id'), (select max(id) from questions));
`
