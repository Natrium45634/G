package sim

// Code — машинный код ошибки/причины. Для каждого кода есть понятный текст (Message).
type Code string

const (
	CodeOK                   Code = ""
	ErrInsufficientAP        Code = "INSUFFICIENT_ACTION_POINTS"
	ErrInsufficientResource  Code = "INSUFFICIENT_RESOURCE"
	ErrOutOfRange            Code = "OUT_OF_RANGE"
	ErrTooClose              Code = "TOO_CLOSE"
	ErrNoLOS                 Code = "NO_LINE_OF_SIGHT"
	ErrTargetInvalid         Code = "TARGET_INVALID"
	ErrTargetDead            Code = "TARGET_DEAD"
	ErrTargetNotVisible      Code = "TARGET_NOT_VISIBLE"
	ErrNoTargets             Code = "NO_TARGETS"
	ErrPathBlocked           Code = "PATH_BLOCKED"
	ErrNoPath                Code = "NO_PATH"
	ErrImpassable            Code = "CELL_IMPASSABLE"
	ErrOccupied              Code = "CELL_OCCUPIED"
	ErrDoorClosed            Code = "DOOR_CLOSED"
	ErrCliff                 Code = "CLIFF"
	ErrOutOfBounds           Code = "OUT_OF_BOUNDS"
	ErrOnCooldown            Code = "ON_COOLDOWN"
	ErrStunned               Code = "UNIT_STUNNED"
	ErrAsleep                Code = "UNIT_ASLEEP"
	ErrRooted                Code = "UNIT_ROOTED"
	ErrUnitDead              Code = "UNIT_DEAD"
	ErrNotOwner              Code = "NOT_YOUR_UNIT"
	ErrUnknownUnit           Code = "UNKNOWN_UNIT"
	ErrUnknownSkill          Code = "UNKNOWN_SKILL"
	ErrSkillNotAvailable     Code = "SKILL_NOT_AVAILABLE"
	ErrNoBasicAttack         Code = "NO_BASIC_ATTACK"
	ErrQueueLocked           Code = "QUEUE_ENDS_WITH_STOP"
	ErrInvalidOrder          Code = "INVALID_ORDER"
	ErrOrderNotFound         Code = "ORDER_NOT_FOUND"
	ErrOrderActive           Code = "ORDER_ALREADY_ACTIVE"
	ErrNothingToUndo         Code = "NOTHING_TO_UNDO"
	ErrNoInteractable        Code = "NO_INTERACTABLE"
	ErrInterrupted           Code = "INTERRUPTED"
	ErrWaitTimeout           Code = "WAIT_TIMEOUT"
	ErrWrongFacing           Code = "WRONG_FACING"
	ErrFriendlyFire          Code = "FRIENDLY_FIRE"
	ErrCancelled             Code = "CANCELLED"
	ErrTargetMoved           Code = "TARGET_MOVED"
	ErrQueueAborted          Code = "QUEUE_ABORTED"
	ErrInvalidMode           Code = "INVALID_MODE"
	ErrCannotCloseOccupied   Code = "DOOR_OCCUPIED"
	ErrBattleOver            Code = "BATTLE_OVER"
	ErrNotEnoughPlanningRoom Code = "QUEUE_TOO_LONG"
)

var messages = map[Code]string{
	ErrInsufficientAP:        "Недостаточно очков действия",
	ErrInsufficientResource:  "Недостаточно ресурса",
	ErrOutOfRange:            "Цель вне дальности",
	ErrTooClose:              "Цель слишком близко",
	ErrNoLOS:                 "Нет линии видимости",
	ErrTargetInvalid:         "Недопустимая цель",
	ErrTargetDead:            "Цель погибла",
	ErrTargetNotVisible:      "Цель не видна",
	ErrNoTargets:             "Нет подходящих целей",
	ErrPathBlocked:           "Путь заблокирован",
	ErrNoPath:                "Нет пути",
	ErrImpassable:            "Непроходимая местность для этого юнита",
	ErrOccupied:              "Клетка занята",
	ErrDoorClosed:            "Дверь закрыта",
	ErrCliff:                 "Слишком крутой перепад высот",
	ErrOutOfBounds:           "Вне карты",
	ErrOnCooldown:            "Умение перезаряжается",
	ErrStunned:               "Юнит оглушён",
	ErrAsleep:                "Юнит спит",
	ErrRooted:                "Юнит обездвижен",
	ErrUnitDead:              "Юнит погиб",
	ErrNotOwner:              "Это не ваш юнит",
	ErrUnknownUnit:           "Юнит не найден",
	ErrUnknownSkill:          "Неизвестное умение",
	ErrSkillNotAvailable:     "Юнит не владеет этим умением",
	ErrNoBasicAttack:         "У юнита нет обычной атаки",
	ErrQueueLocked:           "Очередь завершена приказом «Стоп»",
	ErrInvalidOrder:          "Некорректный приказ",
	ErrOrderNotFound:         "Приказ не найден",
	ErrOrderActive:           "Приказ уже исполняется",
	ErrNothingToUndo:         "Нечего отменять",
	ErrNoInteractable:        "Здесь не с чем взаимодействовать",
	ErrInterrupted:           "Действие прервано",
	ErrWaitTimeout:           "Время ожидания истекло",
	ErrWrongFacing:           "Цель вне сектора атаки",
	ErrFriendlyFire:          "В области поражения есть союзники",
	ErrCancelled:             "Отменено",
	ErrTargetMoved:           "Цель вышла из зоны действия",
	ErrQueueAborted:          "Очередь прервана",
	ErrInvalidMode:           "Неизвестный режим",
	ErrCannotCloseOccupied:   "Нельзя закрыть занятую дверь",
	ErrBattleOver:            "Бой окончен",
	ErrNotEnoughPlanningRoom: "Слишком длинная очередь",
}

// Message — понятный текст для кода.
func Message(c Code) string {
	if m, ok := messages[c]; ok {
		return m
	}
	return string(c)
}

// Issue — проблема или предупреждение с кодом и текстом.
type Issue struct {
	Code Code   `json:"code"`
	Text string `json:"text"`
}

func issue(c Code, detail string) Issue {
	if c == CodeOK {
		return Issue{Text: detail} // пояснение без ошибки
	}
	t := Message(c)
	if detail != "" {
		t += ": " + detail
	}
	return Issue{Code: c, Text: t}
}

// NewIssue — конструктор проблемы для внешних модулей.
func NewIssue(c Code, detail string) Issue { return issue(c, detail) }

func codeFromMap(c string) Code {
	switch c {
	case "":
		return CodeOK
	case "OUT_OF_BOUNDS":
		return ErrOutOfBounds
	case "CELL_IMPASSABLE":
		return ErrImpassable
	case "DOOR_CLOSED":
		return ErrDoorClosed
	case "CLIFF":
		return ErrCliff
	case "CELL_OCCUPIED":
		return ErrOccupied
	case "NO_PATH":
		return ErrNoPath
	}
	return Code(c)
}
