package engine

import "context"

// RequireAdmin skips if bot is not admin.
func RequireAdmin(ctx context.Context, rt *Runtime) error {
	if rt.ChatCtx == nil {
		if err := rt.ProbeChat(ctx); err != nil {
			return err
		}
	}
	if !rt.ChatCtx.BotIsAdmin {
		return Skip("bot is not admin in test chat")
	}
	return nil
}

// RequireCanChangeInfo skips if bot can't change chat info.
func RequireCanChangeInfo(ctx context.Context, rt *Runtime) error {
	if err := RequireAdmin(ctx, rt); err != nil {
		return err
	}
	if !rt.ChatCtx.CanChangeInfo {
		return Skip("bot lacks can_change_info permission")
	}
	return nil
}

// RequireCanRestrict skips if bot can't restrict members.
func RequireCanRestrict(ctx context.Context, rt *Runtime) error {
	if err := RequireAdmin(ctx, rt); err != nil {
		return err
	}
	if !rt.ChatCtx.CanRestrictMembers {
		return Skip("bot lacks can_restrict_members permission")
	}
	return nil
}

// RequireCanDeleteMessages skips if bot can't delete messages.
func RequireCanDeleteMessages(ctx context.Context, rt *Runtime) error {
	if err := RequireAdmin(ctx, rt); err != nil {
		return err
	}
	if !rt.ChatCtx.CanDeleteMessages {
		return Skip("bot lacks can_delete_messages permission")
	}
	return nil
}

// RequireCanManageTopics skips if bot can't manage forum topics.
func RequireCanManageTopics(ctx context.Context, rt *Runtime) error {
	if err := RequireAdmin(ctx, rt); err != nil {
		return err
	}
	if !rt.ChatCtx.CanManageTopics {
		return Skip("bot lacks can_manage_topics permission")
	}
	return nil
}

// RequireForum skips if chat is not a forum.
func RequireForum(ctx context.Context, rt *Runtime) error {
	if rt.ChatCtx == nil {
		if err := rt.ProbeChat(ctx); err != nil {
			return err
		}
	}
	if !rt.ChatCtx.IsForum {
		return Skip("chat is not a forum-enabled supergroup")
	}
	return nil
}

// RequireForumChatID skips if ForumChatID is not configured.
func RequireForumChatID(rt *Runtime) error {
	if rt.ForumChatID == 0 {
		return Skip("TESTBOT_FORUM_CHAT_ID not configured")
	}
	return nil
}

// RequireTestUser skips if TestUserID is not configured.
func RequireTestUser(rt *Runtime) error {
	if rt.TestUserID == 0 {
		return Skip("TESTBOT_TEST_USER_ID not configured")
	}
	return nil
}
