# Форматы данных

Все определения лежат в `engine/data/content/` и встраиваются в игру. Чтобы изменить их
без пересборки, выполните `HexTactics.exe -export-data`: рядом с игрой появится папка
`data/` с теми же файлами; при её наличии игра читает данные оттуда.

Загрузчик строгий: неизвестное поле, неизвестная ссылка (на эффект, характеристику,
умение, класс, поверхность) или неверное значение — ошибка с перечнем причин при запуске.

**Определение ≠ экземпляр.** Класс юнита, умение, эффект, объект — определения. Юнит в бою,
приказ в очереди, активный эффект на юните, ловушка на карте — экземпляры с собственными
текущими параметрами и ссылкой на определение по `id`.

Единицы: время — секунды (переводятся в тики), ОД — дробные числа (внутри — милли-ОД),
проценты — целые числа.

| Файл | Содержимое |
|---|---|
| `rules.json` | правила симуляции и боя |
| `stats.json` | определения характеристик |
| `resources.json` | расходуемые ресурсы (мана, выносливость) |
| `damage_types.json` | типы урона |
| `terrain.json` | поверхности клеток |
| `effects.json` | эффекты (состояния, баффы, дебаффы, ауры) |
| `skills.json` | умения |
| `passives.json` | пассивные навыки |
| `objects.json` | объекты мира (ловушки, отложенные удары) |
| `units.json` | классы юнитов |
| `scenarios/*.json` | карты и расстановки |

## rules.json

```json
{
  "ticks_per_second": 20,          // фиксированный шаг симуляции
  "ap_model": "continuous",        // continuous | cycle
  "cycle_seconds": 6,              // длина цикла для ap_model=cycle
  "queue_mode": "flexible",        // flexible | strict (резервирование ОД)
  "turn_free_while_moving": true,
  "strafe_cost_pct": 150,          // стоимость шага не вперёд при движении без поворота
  "strafe_time_pct": 140,
  "auto_turn_to_attack": true,
  "attack_arc_max_delta": 1,       // сектор атаки атакующего: 0 — только фронт, 1 — + ближние фланги
  "interrupt_on_damage": true,
  "climb_cost_ap": 0.5,            // доплата за уровень подъёма
  "max_climb": 1,                  // больше — обрыв
  "height_hit_bonus": 10,          // % точности за уровень высоты
  "hit_min": 5, "hit_max": 95,
  "evasion_max": 75, "block_max": 90, "resist_max": 90,
  "soft_cover_max": 40,            // предел частичных препятствий на линии выстрела
  "cover_applies_to": ["ranged", "spell"],
  "min_damage": 1,
  "sectors": {                     // индексы: фронт, ближний фланг, дальний фланг, тыл
    "surprise":           [0, 30, 70, 100],
    "evasion":            [100, 90, 70, 50],
    "front_block":        [100, 60, 20, 0],
    "positional_defense": [100, 85, 60, 40]
  },
  "surprise": {"unseen_attacker_bonus": 30, "distracted_bonus": 15,
               "negates_evasion": true, "negates_block": true, "crit_bonus": 10},
  "fail_policies": {"move": "repath", "skill": "skip", ...},   // abort | skip | repath | wait
  "wait_timeout": 3,
  "vision_update_ticks": 4,
  "ai_think_seconds": 0.5,
  "guard_effect": "guarding", "guard_seconds": 6,
  "interact_cost_ap": 1, "interact_time": 0.5,
  "max_events_per_tick": 2000,
  "dangerous_damage_pct": 25       // урон ≥ 25% макс. HP — «опасная атака» (автопауза)
}
```

## stats.json — характеристики

```json
{"id": "evasion", "name": "Уклонение", "group": "defense", "type": "percent",
 "base": 5, "min": 0, "max": 75, "unit": "%", "public": true, "desc": "…"}
```

* `group`: base, resource, combat, defense, movement, perception, resist, vuln, regen, special (для отображения).
* `type`: int | float | percent — способ отображения.
* `base` — значение по умолчанию (класс может переопределить), `min`/`max` — ограничения.
* `public` — видна ли противнику (скрытые не показываются в подсказках; сопротивления открываются после удара).

Используемые ядром: `hp_max, hp_regen, ap_max, ap_regen, move_cost, move_time, turn_cost,
turn_time, action_speed, attack, spell_power, accuracy, crit_chance, crit_damage, armor,
evasion, block, block_power, sight, awareness, stealth, res_*, vuln_*, eres_control, eres_dot,
tenacity, reflect_melee, *_max, *_regen` ресурсов. Любые другие характеристики можно
добавлять — они доступны операциям (`stat`), модификаторам и условиям.

## Модификаторы (в эффектах и пассивках)

```json
{"stat": "attack", "op": "pct", "value": 15,
 "when": {"role": "attacker", "sectors": ["far_flank", "rear"]}}
```

* `op`: `flat` (+значение), `pct` (проценты одного слоя складываются), `mult` (множитель).
* `when` (необязательно) делает модификатор ситуативным: `role` (attacker/defender),
  `sectors` (front, near_flank, far_flank, rear), `terrain`, `attack_kinds`
  (melee/ranged/spell), `hp_below_pct`, `hp_above_pct`, `surprised`.
* Порядок расчёта — см. ARCHITECTURE.md, раздел 6.

## resources.json, damage_types.json

```json
{"id": "mana", "name": "Мана", "max_stat": "mana_max", "regen_stat": "mana_regen", "color": [80,140,255]}
{"id": "fire", "name": "огонь", "armor": false, "resist_stat": "res_fire", "vuln_stat": "vuln_fire", "color": [255,140,40]}
```

Иммунитет к типу урона — тег `immune_damage:<тип>` у класса или пассивного навыка.

## terrain.json — поверхности

```json
{"id": "forest", "name": "Лес", "color": [52,96,52], "pattern": "trees",
 "move_cost": {"foot": 200, "heavy": 250},     // % стоимости шага по классам передвижения; нет ключа — непроходимо
 "blocks_los": false, "los_cover": 15, "cover": 20, "biome": "лес", "desc": "…"}
```

`pattern` — визуальный узор: grass, road, trees, bush, swamp, water, ford, bridge, rock, wall, floor, rubble, sand.

## effects.json — эффекты

```json
{"id": "poisoned", "name": "Отравление", "glyph": "Яд", "color": [130,210,60],
 "kind": "debuff",                 // buff | debuff (упорство сокращает только debuff)
 "tags": ["dot", "poison"],        // для иммунитетов: immune_effect:<тег>
 "flags": [],                      // stunned | rooted | asleep | guarding
 "duration": 4,                    // 0 — бессрочно
 "stacking": "stack", "max_stacks": 3,   // refresh (по умолчанию) | stack | ignore
 "resist_stat": "eres_dot",        // шанс избежать наложения
 "modifiers": [ … ],
 "periodic": {"interval": 1, "ops": [{"op": "damage", "type": "poison", "base": 2}]},
 "absorb": 0,                      // пул поглощения урона
 "redirect": {"pct": 50, "range": 1},    // часть урона — источнику эффекта
 "aura": {"radius": 2, "effect": "rallied", "affects": "allies", "interval": 0.5},
 "break_on_damage": false}
```

## Операции (ops) — общий язык умений, эффектов, ловушек и реакций

| op | Поля | Действие |
|---|---|---|
| `damage` | `type`, `base`, `stat`, `scale` (%), `variance` (±%) | урон: `base + stat×scale/100`; в атаках — через бросок попадания |
| `heal` | `base`, `stat`, `scale` | лечение |
| `apply_effect` | `effect`, `duration` (0 — из определения), `chance` | наложение эффекта |
| `remove_effect` | `effect` | снятие |
| `restore` | `resource` (`ap`, `hp` или id ресурса), `amount` | восстановление |
| `place_object` | `object` | объект в целевой клетке |
| `summon` | `class`, `count` | призыв юнитов стороны источника |

Поле `target`: `target` (по умолчанию), `self`, `attacker`, `source`.

## skills.json — умения

```json
{"id": "fireball", "name": "Огненный шар", "icon": "ОШ", "kind": "area", "color": [255,100,30],
 "ap_cost": 3.5, "costs": {"mana": 25},
 "prep": 1.5, "recovery": 0.4, "cooldown": 8,
 "targeting": "hex",              // self | unit | hex | direction | auto
 "target_filter": "any",          // enemy | ally | ally_or_self | any | empty_hex
 "range_min": 2, "range": 7, "los": true,
 "shape": {"type": "radius", "size": 1},   // single | line | cone(half_angle 30|60) | radius | arc | self; stop_at_wall для линии
 "affects": "all",                // enemies | allies | all (кроме самого) | self
 "requires_facing": true,         // перед применением развернуться к цели/направлению
 "attack": true, "attack_kind": "spell", "accuracy_mod": 0,
 "unevadable": false, "unblockable": true,
 "interruptible": true, "dangerous": true,
 "dash": 0,                       // умение по маршруту: рывок до N клеток перед ударом
 "channel": {"duration": 4, "interval": 1, "effect": "meditating", "ops": [ … ]},
 "no_approach": false,            // запрет автоматического подхода к цели
 "ops": [ … ],
 "ai": "aoe", "ai_min_targets": 2}
```

Демонстрационные умения разных типов: одиночная атака ближнего и дальнего боя (`strike`,
`bow_shot`, `magic_bolt`), дуга (`cleave`), линия (`piercing_arrow`), конус (`fire_cone`),
область вокруг точки с долгой прерываемой подготовкой (`fireball`), отложенный удар
(`meteor`), ловушка-объект (`snare_trap`), автовыбор цели (`quick_shot`), умение по
маршруту (`charge`), защитное (`shield_wall`), аура (`rally`, `war_totem`),
перенаправление урона (`protect`), лечение (`heal`), каналирование (`meditate`),
призыв (`summon_wolf`), контроль (`shield_bash`, `smash`, `poison_dart`, `hex_bolt`).

## passives.json — пассивные навыки

```json
{"id": "riposte", "name": "Ответный удар", "desc": "…",
 "modifiers": [ … ],                              // постоянные и ситуативные
 "move_cost": {"forest": 100, "water": 250},      // переопределение стоимости поверхностей
 "tags": ["immune_effect:control"],
 "triggers": [{"event": "blocked",                // blocked | evaded | damaged | hit_dealt | ally_died | killed
               "attack_kinds": ["melee"], "chance": 0, "cooldown": 2, "once": false,
               "hp_below_pct": 0, "radius": 0,
               "ops": [{"op": "damage", "type": "physical", "base": 4, "stat": "attack", "scale": 50, "target": "attacker"}]}]}
```

## objects.json — объекты мира

```json
{"id": "meteor_mark", "name": "Метка метеора", "glyph": "Мт", "color": [255,80,60],
 "kind": "delayed", "trigger": "timer",          // trigger: enemy_enter | any_enter | timer
 "delay": 3, "radius": 1, "lifetime": 0,
 "hidden_from_enemies": false, "dangerous": true,
 "ops": [ … ]}
```

## units.json — классы юнитов

```json
{"id": "knight", "name": "Рыцарь", "glyph": "Р", "move_class": "heavy", "important": true,
 "stats": {"hp_max": 140, "move_cost": 2, "move_time": 0.8, "block": 45, …},
 "resources": ["stamina"], "basic_attack": "strike",
 "skills": ["cleave", "shield_bash", "shield_wall", "protect"],   // клавиши 1–6 по порядку
 "passives": ["riposte"], "tags": [], "ai": "melee"}             // ai: melee | ranged | caster | support | none
```

Пример различий по ТЗ: тяжёлый воин — 2 ОД за шаг, обычный боец — 1 ОД, разведчик — 0.5 ОД
(`move_cost`); скорость — отдельно (`move_time`, `turn_time`, `action_speed`).

## scenarios/*.json — сценарии

```json
{"id": "river_crossing", "name": "Переправа", "desc": "…", "seed": 20261011,
 "legend": {".": "grass", "T": "forest", "~": "water", "#": "wall", "D": "floor", …},
 "rows":    ["^^^TT....*...~~s..TTT..^^^", …],   // строки карты (смещённые координаты, нечётные строки сдвинуты)
 "heights": ["00000000000000000000000000", …],   // необязательно: высота клетки цифрой
 "features": [{"id": "ruin_door", "kind": "door", "col": 19, "row": 7, "open": false},
              {"id": "ruin_lever", "kind": "lever", "col": 18, "row": 8, "links": ["ruin_door"]}],
 "sides": [{"id": 0, "name": "Отряд", "color": [70,150,255], "controller": "player"},
           {"id": 1, "name": "Гоблины", "color": [230,70,60], "controller": "ai"}],
 "units": [{"class": "knight", "name": "Сэр Освальд", "side": 0, "col": 4, "row": 7, "facing": 0,
            "effects": []}]}       // facing: 0=В, 1=СВ, 2=СЗ, 3=З, 4=ЮЗ, 5=ЮВ
```

## Как добавить новое без изменения ядра

* **Характеристику**: строка в `stats.json`; использовать её в `units.json` (база),
  модификаторах (`stat`) или операциях (`"stat": "luck", "scale": 50`).
* **Умение**: запись в `skills.json` из существующих форм и операций + id в `skills` класса.
* **Эффект**: запись в `effects.json` (модификаторы, флаги, периодические операции, аура, поглощение).
* **Пассивный навык**: модификаторы, переопределение местности, реакции на события.
* **Класс / сценарий**: записи в `units.json` / новый файл в `scenarios/`.
