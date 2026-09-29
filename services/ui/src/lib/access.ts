import type { Permission, Role } from "@/graphql/generated/enums"

// The schema's roles and permissions, as values a form can list. The generated
// types are unions with no runtime form, so the lists are written out here and
// checked against them both ways: a value the schema lacks fails the
// `satisfies`, and one it gained fails the `Missing` check below.
export const ROLES = ["ADMIN"] as const satisfies readonly Role[]

export const PERMISSIONS = [
	"USER_MANAGE",
] as const satisfies readonly Permission[]

type Missing<All, Listed> = Exclude<All, Listed> extends never ? true : never

const rolesComplete: Missing<Role, (typeof ROLES)[number]> = true
const permissionsComplete: Missing<Permission, (typeof PERMISSIONS)[number]> =
	true
void rolesComplete
void permissionsComplete

// The highest rank anyone but the root user can hold, from the schema's
// @constraint on rank.
export const MAX_ASSIGNABLE_RANK = 99

// assignableRank is the highest rank a caller at viewerRank may hand out: a
// user assigns no rank but a lower one.
export function assignableRank(viewerRank: number): number {
	return Math.min(viewerRank - 1, MAX_ASSIGNABLE_RANK)
}

// outranks reports whether a caller at viewerRank may act on someone at rank.
export function outranks(viewerRank: number, rank: number): boolean {
	return viewerRank > rank
}
