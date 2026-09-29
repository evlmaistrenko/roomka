import { graphql } from "@/graphql/generated"

// Every user-management mutation answers with the same fields the list shows,
// so a change made here reads the same as one fetched afresh.
export const MANAGED_USER_FRAGMENT = graphql(`
	fragment ManagedUser on User {
		id
		username
		displayName
		rank
		roles
		directPermissions
		permissions
		passwordSetAt
		createdAt
	}
`)

export const USERS_QUERY = graphql(`
	query Users(
		$filter: UserFilterInput
		$sortBy: UserSort
		$after: String
		$limit: Int!
	) {
		users(filter: $filter, sortBy: $sortBy, after: $after, limit: $limit) {
			items {
				...ManagedUser
			}
			pageInfo {
				endCursor
				hasNextPage
			}
			totalCount
		}
	}
`)

export const USERS_CHANGED_SUBSCRIPTION = graphql(`
	subscription UsersChanged {
		usersChanged
	}
`)

export const CREATE_USER_MUTATION = graphql(`
	mutation CreateUser($input: CreateUserInput!) {
		createUser(input: $input) {
			...ManagedUser
		}
	}
`)

export const SET_USER_ROLES_MUTATION = graphql(`
	mutation SetUserRoles($userId: ID!, $input: SetUserRolesInput!) {
		setUserRoles(userId: $userId, input: $input) {
			...ManagedUser
		}
	}
`)

export const SET_USER_PERMISSIONS_MUTATION = graphql(`
	mutation SetUserPermissions($userId: ID!, $input: SetUserPermissionsInput!) {
		setUserPermissions(userId: $userId, input: $input) {
			...ManagedUser
		}
	}
`)

export const SET_USER_RANK_MUTATION = graphql(`
	mutation SetUserRank($userId: ID!, $input: SetUserRankInput!) {
		setUserRank(userId: $userId, input: $input) {
			...ManagedUser
		}
	}
`)

export const RESET_USER_CREDENTIALS_MUTATION = graphql(`
	mutation ResetUserCredentials($userId: ID!) {
		resetUserCredentials(userId: $userId) {
			...ManagedUser
		}
	}
`)

export const DELETE_USER_MUTATION = graphql(`
	mutation DeleteUser($userId: ID!) {
		deleteUser(userId: $userId)
	}
`)
