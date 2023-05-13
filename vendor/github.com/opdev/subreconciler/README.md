# Subreconciler

![subreconciler logo](assets/subreconciler.png)

**Subreconciler** is a tiny convenience library for [Kubernetes
Operator](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)
developers, adding utilities that improve Operator code readability.

This library encourages you to break your Operator reconciliation logic into
fragments ("subreconciler" functions), which can then be executed (serially, if
needed), and return signals as to whether reconciliation should continue or
halt.

This project is intended to go hand-in-hand with Operator developers consuming
tools like [Operator Framework](https://operatorframework.io/) or
[Kubebuilder](https://github.com/kubernetes-sigs/kubebuilder), and assumes some
degree of familiarity with Operator development using these tools.

## Motivation

The Operator reconciliation workflow generally consists of some incoming event
that triggers your controllers to evaluate said event, and ensure that the
existing state of your resources match the desired state of your specification

In many cases, that workflow involves any number of logical tasks that take
place under one controller, potentially in concert with supplementary
controllers, to ensure application workloads reach their desired state.

The goal of this project is to make the definition, intent, and outcome of each
logical task a bit easier to read, write, parse, and understand.

## Quick Start

The key concepts of this library involve:

- Subreconciler functions

- Flow control

### Subreconciler Functions

The `subreconciler.Fn` type definition represents the smaller, logical units of
work that need to be executed by your controller. You should implement this type
definition in your code as your individual tasks associated with reconciliation.

As an example, you may have:

```Go
// NOTE: the `ctrl` package here refers to the controller-runtime package:
// https://pkg.go.dev/sigs.k8s.io/controller-runtime

func (t *YourReconciler) ensureServiceAccount(c context.Context) (*ctrl.Result, error){
    // omitted for brevity
}

func (t *YourReconciler) ensureRole(c context.Context) (*ctrl.Result, error){
    // omitted for brevity
}

func (t *YourReconciler) ensureRoleBinding(c context.Context) (*ctrl.Result, error){
    // omitted for brevity
}

// etc.
```

All of these match the expected signature of a `subreconciler.Fn`, with their individual goals clearly
defined. Executing these in a logical order might look like:

```Go
func (t *YourReconciler) Reconcile(c context.Context, req ctrl.Request) (ctrl.Result, error){
    // additional code as you need
    subrecs := []subreconciler.Fn{
        t.ensureServiceAccount,
        t.ensureRole,
        t.ensureRoleBinding
    }

    for _, subrec := range subrecs {
        subres, err := subrec(c)
        // More on these the Flow Control section below
        if subreconciler.ShouldHaltOrRequeue(subres, err) {
            return subreconciler.Evaluate(subres, err)
        }
    }
    // additional code as you need

    // When done with all logic, indicate successful reconciliation and do not requeue.
    return subreconciler.Evaluate(subreconciler.DoNotRequeue())
}
```

This structure allows you to build out your order of operations, and add
additional subreconcilers to the execution as needed in a way that's easy to
follow for your future self.

In cases where you may want to pass along the
[Request](https://pkg.go.dev/sigs.k8s.io/controller-runtime#Request) type from
the top-level reconciler, the `subreconciler.FnWithRequest` type definition is
the signature you would want to use instead.

### Flow Control

The previous examples included a few references to flow control functions
available in the `subreconciler` module. The return value of a subreconciler can
indicate to callers whether reconciliation should continue, halt, throw an
error, requeue, etc.

The `subreconciler` module provides helper functions that return the appropriate
return values based on the mentioned case. An example might look like:

```Go
func (t *YourReconciler) doSomeSpecificWork(c context.Context) (*ctrl.Result, error) {
    // In this example, resources being deleted means we should stop our work
    // so we'll return the corresponding subreconciler result.
    if resourceIsBeingDeleted() {
        return subreconciler.DoNotRequeue()
    }

    // In this example, this task can tell us to backoff without throwing an error, in 
    // which case we want to requeue for the work to be done later instead of spamming
    // the reconciliation until the work can be completed.
    backoff, err := doSomeComplicatedWork(c)
    if err != nil {
        // Here, we encountered an error, so we bubble up that error.
        return subreconciler.RequeueWithError(err)
    }

    if backoff {
        // If we were told to back off, we requeue after 10 seconds.
        return subreconciler.RequeueWithDelay(10 * time.Second)
    }

    // We successfully completed this bit of work, so we'll signal our success by indicating
    // that reconciliation can continue.
    return subreconciler.ContinueReconciling()
}
```

It's important that subreconcilers indicate that **reconciliation can continue**
on their completion. This is what allows us to be able to call multiple tasks in
succession.
