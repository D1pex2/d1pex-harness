---
name: 1c-query-optimization
description: "Advanced query patterns for 1C: temporary tables, joins, DCS optimization. Use for complex queries beyond basic rules in query-optimization-tips.md."
---

# 1C Query Optimization Skill (Advanced Patterns)

Продвинутые паттерны оптимизации запросов. Базовые оптимизации (ВЫРАЗИТЬ, ПРЕДСТАВЛЕНИЕ, ВТ вместо подзапросов, ОБЪЕДИНИТЬ ВСЕ, индексы, СКД) - см. [оптимизацию запросов](../composing-1c-queries/references/optimization-and-pitfalls.md). Общие правила запросов и границы проверки - [правила 1С](../1c-development-rules/references/rules.md#запросы).

Примеры с пометками PREFERRED, FAST или SLOW описывают возможные приемы, а не доказанную скорость конкретного запроса. Не переписывай рабочий запрос только из-за такой метки. Для вывода о производительности нужен замер на согласованных данных и проверка эквивалентности, в том числе дублей и поведения внешних соединений. Без замера указывай предположение.

## Validate Query via AI-EDT

Если сервер AI-EDT подключен (инструменты `ai-edt` доступны), проверяй оптимизированный запрос через `validate_query` - убедись, что после рефакторинга нет синтаксических/семантических ошибок. Для запросов СКД: `dcsMode=true`.

## When to Use

Invoke this skill when:
- Working with complex multi-step data processing
- Optimizing joins and subqueries
- Implementing DCS reports
- Processing large datasets in portions

## Temporary Tables - When to Use

Use temporary tables for:
- Complex multi-step data processing
- Joining data from multiple sources
- Reusing intermediate results

### Join vs Subquery (basic choice)

```bsl
// PREFERRED: Join (usually faster)
"ВЫБРАТЬ
|	Заказы.Ссылка КАК Заказ,
|	Контрагенты.ИНН КАК ИНН
|ИЗ
|	Документ.ЗаказКлиента КАК Заказы
|		ЛЕВОЕ СОЕДИНЕНИЕ Справочник.Контрагенты КАК Контрагенты
|		ПО Заказы.Контрагент = Контрагенты.Ссылка"

// AVOID: Subquery in SELECT (N+1 problem)
"ВЫБРАТЬ
|	Заказы.Ссылка КАК Заказ,
|	(ВЫБРАТЬ К.ИНН ИЗ Справочник.Контрагенты КАК К
|	 ГДЕ К.Ссылка = Заказы.Контрагент) КАК ИНН
|ИЗ
|	Документ.ЗаказКлиента КАК Заказы"
```

### Avoid Aggregation in Subqueries

```bsl
// SLOW: Subquery with aggregation
"ВЫБРАТЬ
|	Номенклатура.Ссылка,
|	(ВЫБРАТЬ СУММА(Остатки.Количество) ...) КАК Остаток
|ИЗ
|	Справочник.Номенклатура КАК Номенклатура"

// FAST: Join with pre-aggregated data
"ВЫБРАТЬ
|	Номенклатура.Ссылка КАК Номенклатура,
|	ЕСТЬNULL(Остатки.КоличествоОстаток, 0) КАК Остаток
|ИЗ
|	Справочник.Номенклатура КАК Номенклатура
|		ЛЕВОЕ СОЕДИНЕНИЕ РегистрНакопления.ТоварыНаСкладах.Остатки КАК Остатки
|		ПО Номенклатура.Ссылка = Остатки.Номенклатура"
```

---

**Reference**: [ITS Query Optimization Standards](https://its.1c.ru/db/v8std/browse/13/-1/26/28)
