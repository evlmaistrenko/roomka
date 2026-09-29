import { graphql } from "@/graphql/generated"

// One document for every consumer of the signed-in caller, so urql runs a
// single subscription for all of them.
export const VIEWER_SUBSCRIPTION = graphql(`
	subscription Viewer {
		me {
			user {
				id
				username
				displayName
				rank
				createdAt
			}
			permissions
			preferences {
				key
				value
			}
		}
	}
`)
