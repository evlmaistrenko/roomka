import { LoginForm } from "@/components/login-form"
import { PublicPageLayout } from "@/components/public-page-layout"
import { SetupForm } from "@/components/setup-form"
import { graphql } from "@/graphql/generated"
import { useAuth } from "@/hooks/use-auth"
import { describeError } from "@/lib/graphql-errors"
import { useQuery } from "urql"

const SETUP_REQUIRED_QUERY = graphql(`
	query SetupRequired {
		setupRequired
	}
`)

// SignInPage stands in for whatever page failed with UNAUTHENTICATED. The URL
// stays as it was, so once the sign-in completes that page renders again.
export function SignInPage() {
	const { completeSignIn } = useAuth()
	const [{ data, error }, refetchSetupRequired] = useQuery({
		query: SETUP_REQUIRED_QUERY,
		requestPolicy: "network-only",
	})

	return (
		<PublicPageLayout>
			{error && (
				<p className="text-center text-sm text-destructive">
					{describeError(error)}
				</p>
			)}
			{data === undefined ? null : data.setupRequired ? (
				<SetupForm
					onSignedIn={completeSignIn}
					onAlreadySetUp={() =>
						refetchSetupRequired({ requestPolicy: "network-only" })
					}
				/>
			) : (
				<LoginForm onSignedIn={completeSignIn} />
			)}
		</PublicPageLayout>
	)
}
