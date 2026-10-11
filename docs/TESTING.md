# Тесты и результаты

Запуск всех тестов:

```bash
go test ./...
```

Тесты ядра не требуют графики и выполняются за секунду. Интерфейс проверялся
автоматическими снимками экрана под Xvfb (Linux-сборка) и под Wine (Windows `.exe`),
включая сценарии кликов и клавиш (xdotool): выбор юнита, постановка маршрута, умение в
точке, предпросмотр атаки, подтверждение незавершённой команды, автопауза, справка.

Диагностика баланса: `BATTLE_LOG=1 go test ./engine/sim -run DebugBattleLog -v` печатает
журнал полного боя ИИ против ИИ на «Переправе».

## Результат последнего прогона

```
ok  hextactics/engine/combat
ok  hextactics/engine/data
ok  hextactics/engine/gamemap
ok  hextactics/engine/hex
ok  hextactics/engine/rng
ok  hextactics/engine/sim
ok  hextactics/engine/stats
```

66 тестов: 65 пройдено, 0 провалено, 1 диагностический пропущен (включается переменной окружения).

## Сценарии испытаний из ТЗ

| Сценарий | Тест | Что проверяется |
|---|---|---|
| A. Юнит проходит пять гексов; путь и стоимость верны | `sim.TestScenarioA_FiveHexes`, `gamemap.TestScenarioA_FiveHexPathCost` | проекция: 5 шагов, 5 ОД, остаток 1; фактически списано 5 ОД; время ≈ 5 × 0.55 с; конечная ориентация по движению; стоимость для тяжёлого (10 ОД) и разведчика (2.5 ОД) |
| B. Две точки, направленное умение во второй, очередь продолжается | `sim.TestScenarioB_SkillAtWaypointThenContinue` | умение исполнено из второй точки, линия задела обе цели, затем выполнено движение к третьей точке |
| C. Атаки с разных направлений — верный сектор и модификаторы | `sim.TestScenarioC_DirectionalSectors`, `combat.TestSectorsForAllSixFacings`, `combat.TestSectorModifiersApplied` | все 6 ориентаций × 6 направлений; неожиданность/уклонение/блок по профилю сектора; объяснение «тыл» в журнале |
| D. Изменение проходимости / уход цели — по политике очереди | `sim.TestScenarioD_PassabilityChangeAndTargetLeaves` | дверь закрылась во время пути: `repath` → ожидание → пропуск и переход к следующему приказу; `abort` отменяет остаток очереди; цель ушла во время замаха — удар не достигает цели, очередь продолжается |
| E. Бой при 1× и 4× с одинаковыми исходами — одинаковый результат | `sim.TestScenarioE_SpeedIndependence` | 90 с реального боя (обе стороны под ИИ, > 10 атак), разная частота кадров и скорость → одинаковый хэш состояния и число событий |
| F. Новое умение — без переписывания ядра | `sim.TestScenarioF_NewSkillFromDataOnly` | умение «Ледяной вихрь» описано только данными (конус 120°, урон, обездвиживание): разворот, ОД, ресурсы, атака обеих целей, перезарядка |

## Критерии приёмки

| Критерий | Тесты |
|---|---|
| Шесть корректных соседей, верное расстояние | `hex.TestNeighborsAreSixDistinctAtDistanceOne`, `hex.TestDistance`, `hex.TestDirectionRoundTrip`, `hex.TestDelta` |
| Маршруты учитывают стоимость и препятствия | `gamemap.TestPathAvoidsExpensiveTerrainAndWalls`, `TestOccupiedCellsAndMoveClasses`, `TestHeightsClimbCostAndCliffs`, `TestDoorChangesPassabilityAndLOS` |
| Область движения подсвечивается | `gamemap.TestReachableRespectsBudget` (данные подсветки), проверено снимками интерфейса |
| Пауза останавливает симуляцию; редактирование на паузе | `sim.TestPauseStopsSimulation` |
| Ускорение не меняет правила; таймеры не зависят от FPS | `sim.TestScenarioE_SpeedIndependence`, `TestFrameRateIndependence` |
| События не теряются; автопауза на тике события | `sim.TestAccelerationKeepsAllEvents`, `TestAutoPauseStopsExactlyAtEventTick` |
| Ориентация у каждого юнита, все шесть направлений, сектор верен | `combat.TestSectorsForAllSixFacings`, `TestSectorTieFavoursDefender` |
| Попадание, уклонение и неожиданность независимы | `combat.TestRollsAreIndependent` (статистика 40 000 атак: P(S∧H) ≈ P(S)·P(H); 100% неожиданности ≠ автопопадание) |
| Поворот расходует ОД | `sim.TestTurnCostsAPAndNoSplitExploit` (180° = 1.5 ОД; три поворота по 60° стоят столько же; оглушённый не поворачивается) |
| Несколько точек, номера, линии, умение в точке, стоимость до исполнения | `sim.TestScenarioB_…`, `TestQueueCostExampleAndReservation` (пример ТЗ: движение 2 + поворот + … — итог и остаток) |
| Обработка недопустимых действий, понятные коды ошибок | `sim.TestRejectionReasons`, `TestScenarioD_…`, `data.TestValidationReportsBrokenReferences` |
| Редактирование очереди: изменение точки, перестановка, удаление, отмена изменения и всей очереди, «Стоп» | `sim.TestQueueEditingUndoReorderStop` |
| ОД: непрерывная и цикловая модель, строгое/гибкое резервирование, ожидание ОД, без возврата при отмене | `sim.TestAPRegenContinuousAndCycle`, `TestQueueCostExampleAndReservation`, `TestInsufficientAPWaitsInFlexibleMode`, `TestCancelDoesNotRefund` |
| Новая характеристика без переписывания ядра; порядок модификаторов определён | `stats.TestNewStatWithoutCodeChanges`, `TestOrderOfModifiers`, `TestClampAndCache`, `TestSituationalModifiers` |
| Пассивные навыки получают события через общий механизм | `sim.TestPassivesReactThroughEvents` (ответный удар с перезарядкой, одноразовое «второе дыхание», месть при гибели союзника) |
| Эффекты: периодический урон, аура, поглощение, перенаправление, сопротивление, упорство, иммунитеты | `sim.TestEffectsDotAuraAbsorbRedirectResist` |
| Прерывание: оглушение и урон прерывают подготовку/каналирование | `sim.TestStunInterruptsPreparationAndBlocksActions`, `TestDamageInterruptsInterruptibleSkill`, `TestChannelRestoresAndCanBeInterrupted` |
| Умения разных типов | `TestTrapsDelayedAndSummon` (ловушка, отложенный удар, призыв), `TestChargeDashesAlongRoute` (умение по маршруту), `TestGuardReactsToEnemyEnteringZone` (атака при входе в зону), `TestAttackApproachTurnAndHit` (дойти и атаковать), `TestMoveKeepFacingCostsMore` (движение с заданной ориентацией) |
| Взаимодействие и изменение карты | `sim.TestInteractLeverOpensDoor` |
| Сон и неожиданность | `sim.TestSleepingUnitWakesAndIsSurprised` |
| Сохранение и воспроизведение | `sim.TestSaveLoadContinuesIdentically`, `TestReplayFromCommandLog`, `rng.TestDeterministicAndRestorable` |
| Урон: порядок шагов, иммунитет, минимум | `combat.TestMitigationOrder`, `TestSurpriseNegatesActiveDefenseAndExplains` |
| Данные загружаются и проверяются | `data.TestEmbeddedContentLoadsAndValidates`, `TestValidationReportsBrokenReferences` |
