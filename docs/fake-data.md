# Fake data

This dataset fills a development instance with deliberately varied data. It has
180 courses and 482 students, but no
enrollments. All names, addresses, and offerings are fictional; the
`example.test` teacher addresses cannot deliver mail.

Set up the vocabulary below in the admin UI before importing either CSV.
Categories, periods, and grades do not have CSV importers. IDs must match
exactly: the course import refers to all three kinds of ID, and the student
import refers to grade IDs.

## Categories

Add these on **Admin → Categories**, in any order.

| ID | Name |
| --- | --- |
| `SPORT` | Sports |
| `ART` | Visual Arts |
| `MUSIC` | Music |
| `STEM` | Science and Technology |
| `MAKER` | Design and Making |
| `ACADEMIC` | Academic Enrichment |
| `LANGUAGE` | Languages |
| `CULTURE` | Culture and Heritage |
| `SERVICE` | Community Service |
| `OUTDOOR` | Outdoor Education |
| `WELLNESS` | Wellness |
| `LEADERSHIP` | Leadership |

## Periods

Add these on **Admin → Periods** in the order shown. The application treats
periods as indivisible labels; the times in their names are display text.

| Order | ID | Name |
| ---: | --- | --- |
| 1 | `MON1` | Monday 15:45–16:30 |
| 2 | `MON2` | Monday 16:35–17:20 |
| 3 | `MON3` | Monday 17:25–18:10 |
| 4 | `TUE1` | Tuesday 15:45–16:30 |
| 5 | `TUE2` | Tuesday 16:35–17:20 |
| 6 | `TUE3` | Tuesday 17:25–18:10 |
| 7 | `WED1` | Wednesday 15:45–16:30 |
| 8 | `WED2` | Wednesday 16:35–17:20 |
| 9 | `WED3` | Wednesday 17:25–18:10 |
| 10 | `THU1` | Thursday 15:45–16:30 |
| 11 | `THU2` | Thursday 16:35–17:20 |
| 12 | `THU3` | Thursday 17:25–18:10 |
| 13 | `FRI1` | Friday 15:15–16:00 |
| 14 | `FRI2` | Friday 16:05–16:50 |
| 15 | `FRI3` | Friday 16:55–17:40 |

## Grades

Add these on **Admin → Grades** in the order shown. After creating a grade, set
its enrollment window separately. These deliberately use different budgets and
category requirements. The dates keep the fake windows open for the 2026–27
development year.

| ID | Name | Maximum budgeted periods | Minimum distinct categories | Opens at | Closes at |
| --- | --- | ---: | ---: | --- | --- |
| `Y9` | Year 9 | 5 | 2 | 2026-08-01T00:00:00Z | 2027-07-01T00:00:00Z |
| `Y10` | Year 10 | 7 | 3 | 2026-08-03T00:00:00Z | 2027-07-01T00:00:00Z |
| `Y11` | Year 11 | 9 | 3 | 2026-08-05T00:00:00Z | 2027-07-01T00:00:00Z |
| `Y12` | Year 12 | No cap | 0 | 2026-08-07T00:00:00Z | No closing time |

For additional requirement-group coverage, configure the following optional
requirements:

- **Y9:** at least 1 period across `ART`, `MUSIC`, and `MAKER`.
- **Y10:** at least 2 periods across `SPORT`, `OUTDOOR`, and `WELLNESS`.
- **Y11:** at least 2 periods across `SERVICE` and `LEADERSHIP`, plus at
  least 1 period across `STEM` and `ACADEMIC`.
- **Y12:** no requirement groups.

## Import

Import [fake-courses.csv](fake-courses.csv) from **Admin → Courses**, then
[fake-students.csv](fake-students.csv) from **Admin → Students**. Re-imports
upsert matching IDs; they do not delete records omitted from a file.

The courses intentionally include empty schedules, one-to-five-period
schedules, capacities from 0 to 500, blank and populated teacher details,
several terms and cost labels, invite-only offerings, and varied grade and
legal-sex restrictions. The student roster is intentionally uneven across
grades and includes `F`, `M`, and `X`.

There is intentionally no enrollment dataset. Create enrollments through the
student or admin UI when a scenario needs them.
