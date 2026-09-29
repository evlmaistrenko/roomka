// The English messages. They are also the shape every other language must
// fill: a key missing from a translation fails the type check.
export const en = {
	signIn: {
		title: "Sign in",
		description: "Enter your username and password.",
		username: "Username",
		password: "Password",
		submit: "Sign in",
		wrongCredentials: "Wrong username or password.",
		forgotPassword: "Forgot password?",
		remember: "Remember me",
		rememberHint: "Stay signed in for 30 days instead of one.",
	},
	setup: {
		title: "Set up roomka",
		description: "Create the first account. It becomes the administrator.",
		displayName: "Display name",
		displayNameHint: "3 to 32 characters, starting with a letter.",
		username: "Username",
		usernameHint:
			"3 to 32 lowercase letters and digits, starting with a letter.",
		password: "Password",
		passwordHint: "At least 12 characters.",
		submit: "Create account",
	},
	invalidInput: {
		MALFORMED: "Doesn't match the format described below.",
		TOO_SHORT: "Too short.",
		TOO_LONG: "Too long.",
		TOO_GUESSABLE: "Too easy to guess. Pick a less common password.",
		OUT_OF_RANGE: "Out of range.",
		TAKEN: "Already taken.",
		TARGET_MISSING: "No longer exists.",
	},
	errors: {
		unknown: "Something went wrong. Try again.",
		unreachable: "The server can't be reached.",
		invalidInput: "Check what you entered.",
		unauthenticated: "You need to sign in.",
		stepUpRequired: "Confirm your password to continue.",
		rateLimited: "Too many attempts. Try again in {{seconds}} s.",
		rateLimitedLater: "Too many attempts. Try again later.",
		limitReached: "A limit has been reached.",
		accessDenied: "You don't have access to this.",
	},
	theme: {
		system: "system",
		light: "light",
		dark: "dark",
		current: "Theme: {{theme}}",
		switch: "Theme: {{theme}}, switch to {{next}}",
	},
	language: {
		label: "Language",
	},
	account: {
		menu: "Account",
		signOut: "Sign out",
		settings: "Settings",
	},
	home: {
		greeting: "Hello, {{name}}",
	},
	settings: {
		title: "Settings",
		appearance: "Appearance",
		language: "Language",
		languageHint: "Saved to your account: your other browsers follow it.",
		theme: "Theme",
		themeHint: "Kept in this browser only.",
		themes: {
			system: "System",
			light: "Light",
			dark: "Dark",
		},
	},
	sessions: {
		title: "Sessions",
		description: "Where your account is signed in, and where it was lately.",
		current: "This session",
		others: "Other sessions",
		noOthers: "You haven't signed in anywhere else.",
		signOutOthers: "Sign out everywhere else",
		signOut: "Sign out",
		showMore: "Show {{count}} more",
		showLess: "Show less",
		unknownDevice: "Unknown device",
		statusActive: "Active",
		statusEnded: "Ended",
		signedIn: "signed in {{time}}",
		lastUsed: "last used {{time}}",
		expires: "expires {{time}}",
		endedAt: "ended {{time}}",
	},
	common: {
		cancel: "Cancel",
		done: "Done",
		close: "Close",
		loading: "Loading",
	},
	nav: {
		home: "Home",
		admin: "Administration",
	},
	admin: {
		title: "Administration",
	},
	stepUp: {
		title: "Confirm it's you",
		description:
			"Enter your password to continue. You won't be asked again for 30 minutes.",
		password: "Password",
		submit: "Confirm",
		wrongPassword: "Wrong password.",
	},
	roles: {
		ADMIN: "Administrator",
	},
	roleDescriptions: {
		ADMIN: "Manages users.",
	},
	permissions: {
		USER_MANAGE: "Manage users",
	},
	users: {
		title: "Users",
		description: "Everyone who can sign in to this server.",
		create: "New user",
		search: "Search by username or name",
		sortLabel: "Sort",
		sort: {
			CREATED_DESC: "Newest first",
			CREATED_ASC: "Oldest first",
			USERNAME_ASC: "Username, A to Z",
			USERNAME_DESC: "Username, Z to A",
			RANK_DESC: "Highest rank first",
			RANK_ASC: "Lowest rank first",
		},
		columns: {
			user: "User",
			roles: "Access",
			rank: "Rank",
			status: "Status",
			created: "Created",
			actions: "Actions",
		},
		you: "you",
		active: "Active",
		awaitingPassword: "No password yet",
		empty: "No users match.",
		total: "Total: {{count}}",
		loadMore: "Show more",
		editAccess: "Edit access",
		resetCredentials: "Reset credentials",
		delete: "Delete",
		linkHint:
			"A one-time link for setting a password has been printed in the server console. Pass it on to the user: it is valid for 15 minutes.",
	},
	userForm: {
		displayName: "Display name",
		displayNameHint: "3 to 32 characters, starting with a letter.",
		username: "Username",
		usernameHint:
			"3 to 32 lowercase letters and digits, starting with a letter.",
		roles: "Roles",
		permissions: "Extra permissions",
		permissionsHint: "Granted on top of what the roles give.",
		rank: "Rank",
		rankHint:
			"From 0 to {{max}}. A user can manage only those ranked below them.",
	},
	createUser: {
		title: "New user",
		description:
			"The account starts without a password: the user sets one with a one-time link.",
		submit: "Create",
		created: "{{name}} has been created",
	},
	editAccess: {
		title: "Access for {{name}}",
		description:
			"Changes apply at once. The user's open live views end and restart with the new access.",
		submit: "Save",
	},
	resetCredentials: {
		title: "Reset credentials for {{name}}?",
		description:
			"Their password is cleared and every session they have ends. They set a new password with a one-time link.",
		confirm: "Reset",
		done: "Credentials for {{name}} have been reset",
	},
	deleteUser: {
		title: "Delete {{name}}?",
		description:
			"The account is removed for good, with its sessions and preferences.",
		confirm: "Delete",
	},
	forgotPassword: {
		title: "Forgot your password?",
		description:
			"Enter your username, and a one-time link for setting a new password will be issued.",
		submit: "Get a link",
		requestedTitle: "Ask your administrator for the link",
		requestedDescription:
			"If the account exists, a link for setting a new password has been printed in the server console. It is valid for 15 minutes. Your current password keeps working until you set a new one.",
		toSignIn: "Back to sign in",
	},
	setPassword: {
		title: "Set a password",
		noSecret: "Open this page from the link your administrator gave you.",
		invalid:
			"This link is invalid or has expired. Ask your administrator for a new one.",
		toSignIn: "Back to sign in",
		description: "For {{username}}. The link is valid until {{time}}.",
		password: "New password",
		passwordHint: "At least 12 characters.",
		confirmation: "Repeat the password",
		mismatch: "The passwords don't match.",
		submit: "Set password and sign in",
	},
	notFound: {
		title: "Page not found",
		home: "Go home",
	},
} as const

type Messages<T> = {
	readonly [K in keyof T]: T[K] extends string ? string : Messages<T[K]>
}

export type Translation = Messages<typeof en>
