package mod_mail

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
)

func (m *ModMail) guildMessageCreateListener(event *events.GuildMessageCreate) {
	if event.Message.WebhookID != nil {
		return
	}

	m.Mu.Lock()
	defer m.Mu.Unlock()
	dmID, ok := m.ThreadDMs[event.ChannelID]
	if !ok {
		return
	}
	messageCreate := discord.MessageCreate{
		Embeds: generateEmbeds(event.Message),
		Files:  filesFromAttachments(event.Client(), event.Message.Attachments),
	}

	message, err := event.Client().Rest().CreateMessage(dmID, messageCreate)
	if err != nil {
		event.Client().Logger().Error("failed to create dm message: ", err)
		return
	}
	m.dmMessageIDs[event.Message.ID] = message.ID

}

func (m *ModMail) guildMessageUpdateListener(event *events.GuildMessageUpdate) {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	dmMessageID, ok := m.dmMessageIDs[event.Message.ID]
	if !ok {
		return
	}
	embeds := generateEmbeds(event.Message)
	messageUpdate := discord.MessageUpdate{
		Embeds: &embeds,
		Files:  filesFromAttachments(event.Client(), event.Message.Attachments),
	}
	dmChannelID := m.ThreadDMs[event.ChannelID]
	_, err := event.Client().Rest().UpdateMessage(dmChannelID, dmMessageID, messageUpdate)
	if err != nil {
		event.Client().Logger().Error("failed to update dm message: ", err)
		return
	}

}

func (m *ModMail) guildMessageDeleteListener(event *events.GuildMessageDelete) {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	dmMessageID, ok := m.dmMessageIDs[event.MessageID]
	if !ok {
		return
	}
	delete(m.threadMessageIDs, event.Message.ID)
	dmChannelID := m.ThreadDMs[event.ChannelID]
	if err := event.Client().Rest().DeleteMessage(dmChannelID, dmMessageID); err != nil {
		event.Client().Logger().Error("failed to delete dm message: ", err)
		return
	}

}

func (m *ModMail) guildMemberTypingStartListener(event *events.GuildMemberTypingStart) {
	m.Mu.Lock()
	defer m.Mu.Unlock()

	dmChannelID, ok := m.ThreadDMs[event.ChannelID]
	if !ok {
		return
	}
	if err := event.Client().Rest().SendTyping(dmChannelID); err != nil {
		event.Client().Logger().Error("failed to send dm typing: ", err)
		return

	}
}
