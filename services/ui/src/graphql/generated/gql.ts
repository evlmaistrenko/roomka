/* eslint-disable */
import * as types from './graphql';
import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';

/**
 * Map of all GraphQL operations in the project.
 *
 * This map has several performance disadvantages:
 * 1. It is not tree-shakeable, so it will include all operations in the project.
 * 2. It is not minifiable, so the string of a GraphQL query will be multiple times inside the bundle.
 * 3. It does not support dead code elimination, so it will add unused operations.
 *
 * Therefore it is highly recommended to use the babel or swc plugin for production.
 * Learn more about it here: https://the-guild.dev/graphql/codegen/plugins/presets/preset-client#reducing-bundle-size
 */
type Documents = {
    "\n\tmutation Login($input: LoginInput!) {\n\t\tlogin(input: $input) {\n\t\t\tuser {\n\t\t\t\tid\n\t\t\t}\n\t\t}\n\t}\n": typeof types.LoginDocument,
    "\n\tfragment SessionFields on Session {\n\t\tid\n\t\tcurrent\n\t\tlastUserAgent\n\t\tlastIpAddress\n\t\tcreatedAt\n\t\tlastUsedAt\n\t\texpiresAt\n\t\tendedAt\n\t}\n": typeof types.SessionFieldsFragmentDoc,
    "\n\tsubscription Sessions($limit: Int!) {\n\t\tsessions(sortBy: LAST_USED_DESC, limit: $limit) {\n\t\t\t...SessionFields\n\t\t}\n\t}\n": typeof types.SessionsDocument,
    "\n\tmutation RevokeSession($sessionId: ID!) {\n\t\trevokeSession(sessionId: $sessionId) {\n\t\t\tid\n\t\t}\n\t}\n": typeof types.RevokeSessionDocument,
    "\n\tmutation RevokeOtherSessions {\n\t\trevokeOtherSessions {\n\t\t\tid\n\t\t}\n\t}\n": typeof types.RevokeOtherSessionsDocument,
    "\n\tmutation Setup($input: SetupInput!) {\n\t\tsetup(input: $input) {\n\t\t\tuser {\n\t\t\t\tid\n\t\t\t}\n\t\t}\n\t}\n": typeof types.SetupDocument,
    "\n\tmutation StepUp($input: StepUpInput!) {\n\t\tstepUp(input: $input) {\n\t\t\tsession {\n\t\t\t\tid\n\t\t\t\tstepUpExpiresAt\n\t\t\t}\n\t\t}\n\t}\n": typeof types.StepUpDocument,
    "\n\tmutation Logout {\n\t\tlogout\n\t}\n": typeof types.LogoutDocument,
    "\n\tfragment ManagedUser on User {\n\t\tid\n\t\tusername\n\t\tdisplayName\n\t\trank\n\t\troles\n\t\tdirectPermissions\n\t\tpermissions\n\t\tpasswordSetAt\n\t\tcreatedAt\n\t}\n": typeof types.ManagedUserFragmentDoc,
    "\n\tquery Users(\n\t\t$filter: UserFilterInput\n\t\t$sortBy: UserSort\n\t\t$after: String\n\t\t$limit: Int!\n\t) {\n\t\tusers(filter: $filter, sortBy: $sortBy, after: $after, limit: $limit) {\n\t\t\titems {\n\t\t\t\t...ManagedUser\n\t\t\t}\n\t\t\tpageInfo {\n\t\t\t\tendCursor\n\t\t\t\thasNextPage\n\t\t\t}\n\t\t\ttotalCount\n\t\t}\n\t}\n": typeof types.UsersDocument,
    "\n\tsubscription UsersChanged {\n\t\tusersChanged\n\t}\n": typeof types.UsersChangedDocument,
    "\n\tmutation CreateUser($input: CreateUserInput!) {\n\t\tcreateUser(input: $input) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n": typeof types.CreateUserDocument,
    "\n\tmutation SetUserRoles($userId: ID!, $input: SetUserRolesInput!) {\n\t\tsetUserRoles(userId: $userId, input: $input) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n": typeof types.SetUserRolesDocument,
    "\n\tmutation SetUserPermissions($userId: ID!, $input: SetUserPermissionsInput!) {\n\t\tsetUserPermissions(userId: $userId, input: $input) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n": typeof types.SetUserPermissionsDocument,
    "\n\tmutation SetUserRank($userId: ID!, $input: SetUserRankInput!) {\n\t\tsetUserRank(userId: $userId, input: $input) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n": typeof types.SetUserRankDocument,
    "\n\tmutation ResetUserCredentials($userId: ID!) {\n\t\tresetUserCredentials(userId: $userId) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n": typeof types.ResetUserCredentialsDocument,
    "\n\tmutation DeleteUser($userId: ID!) {\n\t\tdeleteUser(userId: $userId)\n\t}\n": typeof types.DeleteUserDocument,
    "\n\tsubscription Viewer {\n\t\tme {\n\t\t\tuser {\n\t\t\t\tid\n\t\t\t\tusername\n\t\t\t\tdisplayName\n\t\t\t\trank\n\t\t\t\tcreatedAt\n\t\t\t}\n\t\t\tpermissions\n\t\t\tpreferences {\n\t\t\t\tkey\n\t\t\t\tvalue\n\t\t\t}\n\t\t}\n\t}\n": typeof types.ViewerDocument,
    "\n\tmutation SetPreference($key: String!, $value: String) {\n\t\tsetPreference(key: $key, input: { value: $value }) {\n\t\t\tkey\n\t\t}\n\t}\n": typeof types.SetPreferenceDocument,
    "\n\tmutation RequestPasswordReset($username: String!) {\n\t\trequestPasswordReset(username: $username)\n\t}\n": typeof types.RequestPasswordResetDocument,
    "\n\tquery PasswordResetInfo($secret: String!) {\n\t\tpasswordResetInfo(secret: $secret) {\n\t\t\tusername\n\t\t\texpiresAt\n\t\t}\n\t}\n": typeof types.PasswordResetInfoDocument,
    "\n\tmutation SetPassword($secret: String!, $input: SetPasswordInput!) {\n\t\tsetPassword(secret: $secret, input: $input) {\n\t\t\tuser {\n\t\t\t\tid\n\t\t\t}\n\t\t}\n\t}\n": typeof types.SetPasswordDocument,
    "\n\tquery SetupRequired {\n\t\tsetupRequired\n\t}\n": typeof types.SetupRequiredDocument,
};
const documents: Documents = {
    "\n\tmutation Login($input: LoginInput!) {\n\t\tlogin(input: $input) {\n\t\t\tuser {\n\t\t\t\tid\n\t\t\t}\n\t\t}\n\t}\n": types.LoginDocument,
    "\n\tfragment SessionFields on Session {\n\t\tid\n\t\tcurrent\n\t\tlastUserAgent\n\t\tlastIpAddress\n\t\tcreatedAt\n\t\tlastUsedAt\n\t\texpiresAt\n\t\tendedAt\n\t}\n": types.SessionFieldsFragmentDoc,
    "\n\tsubscription Sessions($limit: Int!) {\n\t\tsessions(sortBy: LAST_USED_DESC, limit: $limit) {\n\t\t\t...SessionFields\n\t\t}\n\t}\n": types.SessionsDocument,
    "\n\tmutation RevokeSession($sessionId: ID!) {\n\t\trevokeSession(sessionId: $sessionId) {\n\t\t\tid\n\t\t}\n\t}\n": types.RevokeSessionDocument,
    "\n\tmutation RevokeOtherSessions {\n\t\trevokeOtherSessions {\n\t\t\tid\n\t\t}\n\t}\n": types.RevokeOtherSessionsDocument,
    "\n\tmutation Setup($input: SetupInput!) {\n\t\tsetup(input: $input) {\n\t\t\tuser {\n\t\t\t\tid\n\t\t\t}\n\t\t}\n\t}\n": types.SetupDocument,
    "\n\tmutation StepUp($input: StepUpInput!) {\n\t\tstepUp(input: $input) {\n\t\t\tsession {\n\t\t\t\tid\n\t\t\t\tstepUpExpiresAt\n\t\t\t}\n\t\t}\n\t}\n": types.StepUpDocument,
    "\n\tmutation Logout {\n\t\tlogout\n\t}\n": types.LogoutDocument,
    "\n\tfragment ManagedUser on User {\n\t\tid\n\t\tusername\n\t\tdisplayName\n\t\trank\n\t\troles\n\t\tdirectPermissions\n\t\tpermissions\n\t\tpasswordSetAt\n\t\tcreatedAt\n\t}\n": types.ManagedUserFragmentDoc,
    "\n\tquery Users(\n\t\t$filter: UserFilterInput\n\t\t$sortBy: UserSort\n\t\t$after: String\n\t\t$limit: Int!\n\t) {\n\t\tusers(filter: $filter, sortBy: $sortBy, after: $after, limit: $limit) {\n\t\t\titems {\n\t\t\t\t...ManagedUser\n\t\t\t}\n\t\t\tpageInfo {\n\t\t\t\tendCursor\n\t\t\t\thasNextPage\n\t\t\t}\n\t\t\ttotalCount\n\t\t}\n\t}\n": types.UsersDocument,
    "\n\tsubscription UsersChanged {\n\t\tusersChanged\n\t}\n": types.UsersChangedDocument,
    "\n\tmutation CreateUser($input: CreateUserInput!) {\n\t\tcreateUser(input: $input) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n": types.CreateUserDocument,
    "\n\tmutation SetUserRoles($userId: ID!, $input: SetUserRolesInput!) {\n\t\tsetUserRoles(userId: $userId, input: $input) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n": types.SetUserRolesDocument,
    "\n\tmutation SetUserPermissions($userId: ID!, $input: SetUserPermissionsInput!) {\n\t\tsetUserPermissions(userId: $userId, input: $input) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n": types.SetUserPermissionsDocument,
    "\n\tmutation SetUserRank($userId: ID!, $input: SetUserRankInput!) {\n\t\tsetUserRank(userId: $userId, input: $input) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n": types.SetUserRankDocument,
    "\n\tmutation ResetUserCredentials($userId: ID!) {\n\t\tresetUserCredentials(userId: $userId) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n": types.ResetUserCredentialsDocument,
    "\n\tmutation DeleteUser($userId: ID!) {\n\t\tdeleteUser(userId: $userId)\n\t}\n": types.DeleteUserDocument,
    "\n\tsubscription Viewer {\n\t\tme {\n\t\t\tuser {\n\t\t\t\tid\n\t\t\t\tusername\n\t\t\t\tdisplayName\n\t\t\t\trank\n\t\t\t\tcreatedAt\n\t\t\t}\n\t\t\tpermissions\n\t\t\tpreferences {\n\t\t\t\tkey\n\t\t\t\tvalue\n\t\t\t}\n\t\t}\n\t}\n": types.ViewerDocument,
    "\n\tmutation SetPreference($key: String!, $value: String) {\n\t\tsetPreference(key: $key, input: { value: $value }) {\n\t\t\tkey\n\t\t}\n\t}\n": types.SetPreferenceDocument,
    "\n\tmutation RequestPasswordReset($username: String!) {\n\t\trequestPasswordReset(username: $username)\n\t}\n": types.RequestPasswordResetDocument,
    "\n\tquery PasswordResetInfo($secret: String!) {\n\t\tpasswordResetInfo(secret: $secret) {\n\t\t\tusername\n\t\t\texpiresAt\n\t\t}\n\t}\n": types.PasswordResetInfoDocument,
    "\n\tmutation SetPassword($secret: String!, $input: SetPasswordInput!) {\n\t\tsetPassword(secret: $secret, input: $input) {\n\t\t\tuser {\n\t\t\t\tid\n\t\t\t}\n\t\t}\n\t}\n": types.SetPasswordDocument,
    "\n\tquery SetupRequired {\n\t\tsetupRequired\n\t}\n": types.SetupRequiredDocument,
};

/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 *
 *
 * @example
 * ```ts
 * const query = graphql(`query GetUser($id: ID!) { user(id: $id) { name } }`);
 * ```
 *
 * The query argument is unknown!
 * Please regenerate the types.
 */
export function graphql(source: string): unknown;

/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation Login($input: LoginInput!) {\n\t\tlogin(input: $input) {\n\t\t\tuser {\n\t\t\t\tid\n\t\t\t}\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation Login($input: LoginInput!) {\n\t\tlogin(input: $input) {\n\t\t\tuser {\n\t\t\t\tid\n\t\t\t}\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tfragment SessionFields on Session {\n\t\tid\n\t\tcurrent\n\t\tlastUserAgent\n\t\tlastIpAddress\n\t\tcreatedAt\n\t\tlastUsedAt\n\t\texpiresAt\n\t\tendedAt\n\t}\n"): (typeof documents)["\n\tfragment SessionFields on Session {\n\t\tid\n\t\tcurrent\n\t\tlastUserAgent\n\t\tlastIpAddress\n\t\tcreatedAt\n\t\tlastUsedAt\n\t\texpiresAt\n\t\tendedAt\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tsubscription Sessions($limit: Int!) {\n\t\tsessions(sortBy: LAST_USED_DESC, limit: $limit) {\n\t\t\t...SessionFields\n\t\t}\n\t}\n"): (typeof documents)["\n\tsubscription Sessions($limit: Int!) {\n\t\tsessions(sortBy: LAST_USED_DESC, limit: $limit) {\n\t\t\t...SessionFields\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation RevokeSession($sessionId: ID!) {\n\t\trevokeSession(sessionId: $sessionId) {\n\t\t\tid\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation RevokeSession($sessionId: ID!) {\n\t\trevokeSession(sessionId: $sessionId) {\n\t\t\tid\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation RevokeOtherSessions {\n\t\trevokeOtherSessions {\n\t\t\tid\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation RevokeOtherSessions {\n\t\trevokeOtherSessions {\n\t\t\tid\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation Setup($input: SetupInput!) {\n\t\tsetup(input: $input) {\n\t\t\tuser {\n\t\t\t\tid\n\t\t\t}\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation Setup($input: SetupInput!) {\n\t\tsetup(input: $input) {\n\t\t\tuser {\n\t\t\t\tid\n\t\t\t}\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation StepUp($input: StepUpInput!) {\n\t\tstepUp(input: $input) {\n\t\t\tsession {\n\t\t\t\tid\n\t\t\t\tstepUpExpiresAt\n\t\t\t}\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation StepUp($input: StepUpInput!) {\n\t\tstepUp(input: $input) {\n\t\t\tsession {\n\t\t\t\tid\n\t\t\t\tstepUpExpiresAt\n\t\t\t}\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation Logout {\n\t\tlogout\n\t}\n"): (typeof documents)["\n\tmutation Logout {\n\t\tlogout\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tfragment ManagedUser on User {\n\t\tid\n\t\tusername\n\t\tdisplayName\n\t\trank\n\t\troles\n\t\tdirectPermissions\n\t\tpermissions\n\t\tpasswordSetAt\n\t\tcreatedAt\n\t}\n"): (typeof documents)["\n\tfragment ManagedUser on User {\n\t\tid\n\t\tusername\n\t\tdisplayName\n\t\trank\n\t\troles\n\t\tdirectPermissions\n\t\tpermissions\n\t\tpasswordSetAt\n\t\tcreatedAt\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tquery Users(\n\t\t$filter: UserFilterInput\n\t\t$sortBy: UserSort\n\t\t$after: String\n\t\t$limit: Int!\n\t) {\n\t\tusers(filter: $filter, sortBy: $sortBy, after: $after, limit: $limit) {\n\t\t\titems {\n\t\t\t\t...ManagedUser\n\t\t\t}\n\t\t\tpageInfo {\n\t\t\t\tendCursor\n\t\t\t\thasNextPage\n\t\t\t}\n\t\t\ttotalCount\n\t\t}\n\t}\n"): (typeof documents)["\n\tquery Users(\n\t\t$filter: UserFilterInput\n\t\t$sortBy: UserSort\n\t\t$after: String\n\t\t$limit: Int!\n\t) {\n\t\tusers(filter: $filter, sortBy: $sortBy, after: $after, limit: $limit) {\n\t\t\titems {\n\t\t\t\t...ManagedUser\n\t\t\t}\n\t\t\tpageInfo {\n\t\t\t\tendCursor\n\t\t\t\thasNextPage\n\t\t\t}\n\t\t\ttotalCount\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tsubscription UsersChanged {\n\t\tusersChanged\n\t}\n"): (typeof documents)["\n\tsubscription UsersChanged {\n\t\tusersChanged\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation CreateUser($input: CreateUserInput!) {\n\t\tcreateUser(input: $input) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation CreateUser($input: CreateUserInput!) {\n\t\tcreateUser(input: $input) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation SetUserRoles($userId: ID!, $input: SetUserRolesInput!) {\n\t\tsetUserRoles(userId: $userId, input: $input) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation SetUserRoles($userId: ID!, $input: SetUserRolesInput!) {\n\t\tsetUserRoles(userId: $userId, input: $input) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation SetUserPermissions($userId: ID!, $input: SetUserPermissionsInput!) {\n\t\tsetUserPermissions(userId: $userId, input: $input) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation SetUserPermissions($userId: ID!, $input: SetUserPermissionsInput!) {\n\t\tsetUserPermissions(userId: $userId, input: $input) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation SetUserRank($userId: ID!, $input: SetUserRankInput!) {\n\t\tsetUserRank(userId: $userId, input: $input) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation SetUserRank($userId: ID!, $input: SetUserRankInput!) {\n\t\tsetUserRank(userId: $userId, input: $input) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation ResetUserCredentials($userId: ID!) {\n\t\tresetUserCredentials(userId: $userId) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation ResetUserCredentials($userId: ID!) {\n\t\tresetUserCredentials(userId: $userId) {\n\t\t\t...ManagedUser\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation DeleteUser($userId: ID!) {\n\t\tdeleteUser(userId: $userId)\n\t}\n"): (typeof documents)["\n\tmutation DeleteUser($userId: ID!) {\n\t\tdeleteUser(userId: $userId)\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tsubscription Viewer {\n\t\tme {\n\t\t\tuser {\n\t\t\t\tid\n\t\t\t\tusername\n\t\t\t\tdisplayName\n\t\t\t\trank\n\t\t\t\tcreatedAt\n\t\t\t}\n\t\t\tpermissions\n\t\t\tpreferences {\n\t\t\t\tkey\n\t\t\t\tvalue\n\t\t\t}\n\t\t}\n\t}\n"): (typeof documents)["\n\tsubscription Viewer {\n\t\tme {\n\t\t\tuser {\n\t\t\t\tid\n\t\t\t\tusername\n\t\t\t\tdisplayName\n\t\t\t\trank\n\t\t\t\tcreatedAt\n\t\t\t}\n\t\t\tpermissions\n\t\t\tpreferences {\n\t\t\t\tkey\n\t\t\t\tvalue\n\t\t\t}\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation SetPreference($key: String!, $value: String) {\n\t\tsetPreference(key: $key, input: { value: $value }) {\n\t\t\tkey\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation SetPreference($key: String!, $value: String) {\n\t\tsetPreference(key: $key, input: { value: $value }) {\n\t\t\tkey\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation RequestPasswordReset($username: String!) {\n\t\trequestPasswordReset(username: $username)\n\t}\n"): (typeof documents)["\n\tmutation RequestPasswordReset($username: String!) {\n\t\trequestPasswordReset(username: $username)\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tquery PasswordResetInfo($secret: String!) {\n\t\tpasswordResetInfo(secret: $secret) {\n\t\t\tusername\n\t\t\texpiresAt\n\t\t}\n\t}\n"): (typeof documents)["\n\tquery PasswordResetInfo($secret: String!) {\n\t\tpasswordResetInfo(secret: $secret) {\n\t\t\tusername\n\t\t\texpiresAt\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tmutation SetPassword($secret: String!, $input: SetPasswordInput!) {\n\t\tsetPassword(secret: $secret, input: $input) {\n\t\t\tuser {\n\t\t\t\tid\n\t\t\t}\n\t\t}\n\t}\n"): (typeof documents)["\n\tmutation SetPassword($secret: String!, $input: SetPasswordInput!) {\n\t\tsetPassword(secret: $secret, input: $input) {\n\t\t\tuser {\n\t\t\t\tid\n\t\t\t}\n\t\t}\n\t}\n"];
/**
 * The graphql function is used to parse GraphQL queries into a document that can be used by GraphQL clients.
 */
export function graphql(source: "\n\tquery SetupRequired {\n\t\tsetupRequired\n\t}\n"): (typeof documents)["\n\tquery SetupRequired {\n\t\tsetupRequired\n\t}\n"];

export function graphql(source: string) {
  return (documents as any)[source] ?? {};
}

export type DocumentType<TDocumentNode extends DocumentNode<any, any>> = TDocumentNode extends DocumentNode<  infer TType,  any>  ? TType  : never;