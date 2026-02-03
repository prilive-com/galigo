package tg

// InputChecklist represents a checklist to be sent.
type InputChecklist struct {
	Title                    string               `json:"title"`
	ParseMode                string               `json:"parse_mode,omitempty"`
	TitleEntities            []MessageEntity      `json:"title_entities,omitempty"`
	Tasks                    []InputChecklistTask `json:"tasks"`
	OthersCanAddTasks        bool                 `json:"others_can_add_tasks,omitempty"`
	OthersCanMarkTasksAsDone bool                 `json:"others_can_mark_tasks_as_done,omitempty"`
}

// InputChecklistTask represents a task in a checklist to be sent.
type InputChecklistTask struct {
	ID           int             `json:"id"`
	Text         string          `json:"text"`
	ParseMode    string          `json:"parse_mode,omitempty"`
	TextEntities []MessageEntity `json:"text_entities,omitempty"`
}

// Checklist represents a checklist in a received message.
type Checklist struct {
	Title                    string          `json:"title"`
	TitleEntities            []MessageEntity `json:"title_entities,omitempty"`
	Tasks                    []ChecklistTask `json:"tasks"`
	OthersCanAddTasks        bool            `json:"others_can_add_tasks"`
	OthersCanMarkTasksAsDone bool            `json:"others_can_mark_tasks_as_done"`
}

// ChecklistTask represents a task in a received checklist.
type ChecklistTask struct {
	ID            int             `json:"id"`
	Text          string          `json:"text"`
	TextEntities  []MessageEntity `json:"text_entities,omitempty"`
	IsDone        bool            `json:"is_done"`
	CompletedByID int64           `json:"completed_by_id,omitempty"`
}
