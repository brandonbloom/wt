package cli

type workflowExpectations struct {
	PRsExpected bool
}

func workflowExpectationsForProject(compareCtx defaultBranchCompareContext) workflowExpectations {
	return workflowExpectations{PRsExpected: compareCtx.PRsExpected}
}

