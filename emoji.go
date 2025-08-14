// [Emojis] は
// /emoji から
// https://mholt.github.io/json-to-go/ で生成後、修正
package miutil

type Emojis struct {
	Emojis []Emoji `json:"emojis"`
}

type Emoji struct {
	ID                                         string   `json:"id"`
	Aliases                                    []string `json:"aliases"`
	Name                                       string   `json:"name"`
	Category                                   string   `json:"category"`
	Host                                       any      `json:"host"`
	URL                                        string   `json:"url"`
	License                                    string   `json:"license"`
	IsSensitive                                bool     `json:"isSensitive"`
	LocalOnly                                  bool     `json:"localOnly"`
	RoleIdsThatCanBeUsedThisEmojiAsReaction    []any    `json:"roleIdsThatCanBeUsedThisEmojiAsReaction"`
	RoleIdsThatCanNotBeUsedThisEmojiAsReaction []any    `json:"roleIdsThatCanNotBeUsedThisEmojiAsReaction"`
}
